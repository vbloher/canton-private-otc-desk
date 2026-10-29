import { describe, expect, it } from "vitest";
import { unwrapSeed, wrapSeed } from "./blob";

const random = (n: number): Uint8Array<ArrayBuffer> => crypto.getRandomValues(new Uint8Array(n));
const fromB64 = (b64: string): Uint8Array<ArrayBuffer> => Uint8Array.from(atob(b64), (c) => c.charCodeAt(0));

async function specDecrypt(blob: string, prfSecret: Uint8Array<ArrayBuffer>): Promise<Uint8Array> {
  const ikm = await crypto.subtle.importKey("raw", prfSecret, "HKDF", false, ["deriveKey"]);
  const kek = await crypto.subtle.deriveKey(
    { name: "HKDF", hash: "SHA-256", salt: new TextEncoder().encode("otc-desk-kek-v1"), info: new Uint8Array(0) },
    ikm,
    { name: "AES-GCM", length: 256 },
    false,
    ["decrypt"],
  );
  const bytes = fromB64(blob);
  const plain = await crypto.subtle.decrypt({ name: "AES-GCM", iv: bytes.subarray(0, 12) }, kek, bytes.subarray(12));
  return new Uint8Array(plain);
}

describe("wrapSeed / unwrapSeed", () => {
  it("round-trips a seed under the same PRF secret", async () => {
    const seed = random(32);
    const secret = random(32);

    const blob = await wrapSeed(seed, secret);

    expect(await unwrapSeed(blob, secret)).toEqual(seed);
  });

  it("fails with any other PRF secret", async () => {
    const blob = await wrapSeed(random(32), random(32));

    await expect(unwrapSeed(blob, random(32))).rejects.toThrow();
  });

  it("writes base64(iv ‖ ciphertext ‖ tag) that the spec's recipe opens", async () => {
    const seed = random(32);
    const secret = random(32);

    const blob = await wrapSeed(seed, secret);

    expect(fromB64(blob)).toHaveLength(12 + 32 + 16);
    expect(await specDecrypt(blob, secret)).toEqual(seed);
  });

  it("uses a fresh IV for every wrap", async () => {
    const seed = random(32);
    const secret = random(32);

    const [a, b] = await Promise.all([wrapSeed(seed, secret), wrapSeed(seed, secret)]);

    expect(a).not.toBe(b);
  });

  it("rejects a blob too short to hold an IV and a tag", async () => {
    await expect(unwrapSeed(btoa("short"), random(32))).rejects.toThrow(/too short/);
  });
});
