import { describe, expect, it } from "vitest";
import { PasskeyCancelledError, PrfUnsupportedError } from "@/application/errors";
import { unwrapSeed } from "./blob";
import { createPasskeyVault, type WebAuthnEnv } from "./prf";

const RAW_ID = Uint8Array.of(0xfb, 0xff, 0x01, 0x02); // base64url "-_8BAg"
const PRF_OUTPUT = new Uint8Array(32).fill(0x42);

type Options = { publicKey: Record<string, unknown> };

/** A fake authenticator: records the options it was given and answers like a browser would. */
function fakeEnv(opts: { prf?: boolean; refuse?: boolean; capabilities?: Record<string, boolean> } = {}) {
  const seen: { create?: Options; get?: Options } = {};
  const answer = () => ({
    rawId: RAW_ID.buffer.slice(0),
    getClientExtensionResults: () => (opts.prf === false ? {} : { prf: { results: { first: PRF_OUTPUT.buffer.slice(0) } } }),
  });
  const refuse = () => {
    throw new DOMException("The operation either timed out or was not allowed.", "NotAllowedError");
  };
  const env: WebAuthnEnv = {
    rpId: "desk.test",
    credentials: {
      create: async (o) => {
        seen.create = o as unknown as Options;
        if (opts.refuse === true) refuse();
        return answer() as unknown as Credential;
      },
      get: async (o) => {
        seen.get = o as unknown as Options;
        if (opts.refuse === true) refuse();
        return answer() as unknown as Credential;
      },
    },
    capabilities: async () => opts.capabilities,
  };
  return { env, seen };
}

const text = (v: unknown): string => new TextDecoder().decode(v as Uint8Array);

describe("createPasskeyVault", () => {
  it("creates a discoverable passkey for this host, named after the desk, with PRF", async () => {
    const { env, seen } = fakeEnv();

    const credentialId = await createPasskeyVault(env).createPasskey("Alice");

    expect(credentialId).toBe("-_8BAg");
    const pk = seen.create?.publicKey;
    expect(pk?.rp).toEqual({ name: "Private OTC Desk", id: "desk.test" });
    expect(pk?.authenticatorSelection).toMatchObject({ residentKey: "required", userVerification: "required" });
    expect(pk?.extensions).toHaveProperty("prf");
    expect(pk?.user).toMatchObject({ name: "Alice", displayName: "Alice" });
  });

  it("unlocks with the spec's PRF salt and wraps the seed with the PRF output", async () => {
    const { env, seen } = fakeEnv();

    const unlocked = await createPasskeyVault(env).unlock("-_8BAg");

    const pk = seen.get?.publicKey as { extensions: { prf: { eval: { first: unknown } } }; rpId: string };
    expect(text(pk.extensions.prf.eval.first)).toBe("otc-desk-prf-salt-v1");
    expect(pk.rpId).toBe("desk.test");
    expect(unlocked.credentialId).toBe("-_8BAg");
    const seed = new Uint8Array(32).fill(7);
    expect(await unwrapSeed(await unlocked.wrap(seed), PRF_OUTPUT)).toEqual(seed);
    expect(await unlocked.unwrap(await unlocked.wrap(seed))).toEqual(seed);
  });

  it("offers only the remembered passkey, or every discoverable one", async () => {
    const remembered = fakeEnv();
    await createPasskeyVault(remembered.env).unlock("-_8BAg");
    const allowed = remembered.seen.get?.publicKey.allowCredentials as { id: Uint8Array }[];
    expect(allowed.map((c) => [...c.id])).toEqual([[0xfb, 0xff, 0x01, 0x02]]);

    const any = fakeEnv();
    await createPasskeyVault(any.env).unlock();
    expect(any.seen.get?.publicKey.allowCredentials).toEqual([]);
  });

  it("says so when the authenticator returns no PRF output", async () => {
    await expect(createPasskeyVault(fakeEnv({ prf: false }).env).unlock()).rejects.toBeInstanceOf(PrfUnsupportedError);
  });

  it("turns a dismissed prompt into a cancellation", async () => {
    const vault = createPasskeyVault(fakeEnv({ refuse: true }).env);
    await expect(vault.createPasskey("Alice")).rejects.toBeInstanceOf(PasskeyCancelledError);
    await expect(vault.unlock()).rejects.toBeInstanceOf(PasskeyCancelledError);
  });

  it("reports what the browser can do", async () => {
    expect(await createPasskeyVault(undefined).support()).toBe("none");
    expect(await createPasskeyVault(fakeEnv({ capabilities: { "extension:prf": false } }).env).support()).toBe("no-prf");
    expect(await createPasskeyVault(fakeEnv({ capabilities: { "extension:prf": true } }).env).support()).toBe("supported");
    expect(await createPasskeyVault(fakeEnv().env).support()).toBe("unknown");
  });
});
