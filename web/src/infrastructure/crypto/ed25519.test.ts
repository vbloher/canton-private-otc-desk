import { describe, expect, it } from "vitest";
import { ed25519Keys } from "./ed25519";

const fromHex = (h: string): Uint8Array => Uint8Array.from(h.match(/../g) ?? [], (b) => parseInt(b, 16));
const toHex = (b: Uint8Array): string => [...b].map((x) => x.toString(16).padStart(2, "0")).join("");

const SEED = "9d61b19deffd5a60ba844af492ec2cc44449c5697b326919703bac031cae7f60";
const PUBLIC = "d75a980182b10ab7d54bfed3c964073a0ee172f3daa62325af021a68f707511a";
const SIGNATURE =
  "e5564300c360ac729086e2cc806e828a84877f1eb8e5d974d873e065224901555fb8821590a33bacc61e39701cf9b46bd25bf5f0595bbe24655141438e7a100b";

describe("ed25519Keys", () => {
  it("derives the raw 32-byte public key of a seed", async () => {
    expect(toHex(await ed25519Keys.publicKey(fromHex(SEED)))).toBe(PUBLIC);
  });

  it("signs raw bytes as RFC 8032 Ed25519", async () => {
    const signature = await ed25519Keys.signer(fromHex(SEED)).sign(new Uint8Array(0));
    expect(toHex(signature)).toBe(SIGNATURE);
  });

  it("generates a fresh 32-byte seed every time", () => {
    const a = ed25519Keys.generateSeed();
    const b = ed25519Keys.generateSeed();
    expect(a).toHaveLength(32);
    expect(toHex(a)).not.toBe(toHex(b));
  });
});
