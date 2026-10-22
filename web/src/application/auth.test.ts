import { describe, expect, it } from "vitest";
import type { Me } from "@/domain/types";
import { createAuthFlows } from "./auth";
import { KeyMismatchError, PasskeyCancelledError, UnknownPasskeyError } from "./errors";
import type { AuthApi, CredentialStore, KeyVault, LoginChallenge, PartyKeys, UnlockedPasskey } from "./ports";

const enc = new TextEncoder();
const dec = new TextDecoder();
const hex = (b: Uint8Array): string => [...b].map((x) => x.toString(16).padStart(2, "0")).join("");

const ME: Me = {
  id: "u-1",
  partyId: "alice-3f9a1c::1220ab",
  hint: "alice-3f9a1c",
  displayName: "Alice",
  kind: "passkey",
  makerMode: false,
};

/** The wallet behind the passkey "cred-bob". */
const BOB: Me = { ...ME, id: "u-2", partyId: "bob-77aa01::1220cd", hint: "bob-77aa01", displayName: "Bob" };

/** Seeds are 32 copies of one byte; the "public key" is "pk" + that byte; a signature is "<byte>:<message>". */
const keys = (seedByte = 9): PartyKeys => ({
  generateSeed: () => new Uint8Array(32).fill(seedByte),
  publicKey: async (seed) => enc.encode(`pk${seed[0] ?? -1}`),
  signer: (seed) => ({ sign: async (m) => enc.encode(`${seed[0] ?? -1}:${dec.decode(m)}`) }),
});

/** A passkey's PRF key "wraps" a seed as "blob:<hex>" and opens only its own blobs. */
function passkey(credentialId: string): UnlockedPasskey {
  return {
    credentialId,
    wrap: async (seed) => `blob:${hex(seed)}`,
    unwrap: async (blob) => {
      const m = /^blob:([0-9a-f]+)$/.exec(blob);
      if (m?.[1] === undefined) throw new Error("bad blob");
      return Uint8Array.from(m[1].match(/../g) ?? [], (h) => parseInt(h, 16));
    },
  };
}

function vault(opts: { created?: string; discovered?: string; failUnlockOnce?: boolean } = {}) {
  const log: string[] = [];
  let failNext = opts.failUnlockOnce ?? false;
  const v: KeyVault = {
    support: async () => "supported",
    createPasskey: async (name) => {
      log.push(`create ${name}`);
      return opts.created ?? "cred-new";
    },
    unlock: async (id) => {
      log.push(`unlock ${id ?? "(any)"}`);
      if (failNext) {
        failNext = false;
        throw new PasskeyCancelledError("passkey sign-in");
      }
      return passkey(id ?? opts.discovered ?? "cred-found");
    },
  };
  return { vault: v, log };
}

function api(challenge: Partial<LoginChallenge> = {}) {
  const log: string[] = [];
  const a: AuthApi = {
    registerStart: async ({ displayName, publicKey }) => {
      log.push(`start ${displayName} ${dec.decode(publicKey)}`);
      return { registrationId: "reg-1", partyId: ME.partyId, multiHash: enc.encode("multihash") };
    },
    registerComplete: async ({ registrationId, signature, credentialId, encryptedBlob }) => {
      log.push(`complete ${registrationId} ${dec.decode(signature)} ${credentialId} ${encryptedBlob}`);
      return ME;
    },
    createChallenge: async (credentialId) => {
      log.push(`challenge ${credentialId}`);
      return {
        challenge: "otc-desk:abc",
        encryptedBlob: `blob:${hex(new Uint8Array(32).fill(5))}`,
        publicKey: enc.encode("pk5"),
        ...challenge,
      };
    },
    login: async ({ challenge: c, signature, credentialId }) => {
      log.push(`login ${c} ${dec.decode(signature)} ${credentialId}`);
      return credentialId === "cred-bob" ? BOB : ME;
    },
    logout: async () => {
      log.push("logout");
    },
    me: async () => ME,
    setMakerMode: async () => ME,
    makers: async () => [],
  };
  return { api: a, log };
}

function store(initial: Record<string, string> = {}): CredentialStore & { data: Record<string, string> } {
  const data = { ...initial };
  return { data, get: (id) => data[id], set: (id, cred) => void (data[id] = cred) };
}

describe("createWallet", () => {
  it("creates a passkey, wraps a fresh key with it and registers the party", async () => {
    const v = vault({ created: "cred-1" });
    const a = api();
    const credentials = store();
    const steps: string[] = [];
    const flows = createAuthFlows({ auth: a.api, vault: v.vault, keys: keys(9), credentials });

    const wallet = await flows.createWallet("  Alice ", (s) => steps.push(s));

    expect(v.log).toEqual(["create Alice", "unlock cred-1"]);
    expect(a.log).toEqual([
      "start Alice pk9",
      `complete reg-1 9:multihash cred-1 blob:${"09".repeat(32)}`,
    ]);
    expect(steps).toEqual(["passkey", "unlock", "party"]);
    expect(wallet.me).toEqual(ME);
    expect(wallet.seed).toEqual(new Uint8Array(32).fill(9));
    expect(credentials.data).toEqual({ "u-1": "cred-1" });
  });

  it("reuses the passkey of an attempt that did not finish", async () => {
    const v = vault({ created: "cred-1", failUnlockOnce: true });
    const flows = createAuthFlows({ auth: api().api, vault: v.vault, keys: keys(), credentials: store() });

    await expect(flows.createWallet("Alice")).rejects.toBeInstanceOf(PasskeyCancelledError);
    await flows.createWallet("Alice");

    expect(v.log).toEqual(["create Alice", "unlock cred-1", "unlock cred-1"]);
  });
});

describe("connectWallet", () => {
  it("opens the wallet of any passkey and signs the challenge to sign in", async () => {
    const v = vault({ discovered: "cred-7" });
    const a = api();
    const credentials = store();
    const flows = createAuthFlows({ auth: a.api, vault: v.vault, keys: keys(), credentials });

    const wallet = await flows.connectWallet();

    expect(v.log).toEqual(["unlock (any)"]);
    expect(a.log).toEqual(["challenge cred-7", "login otc-desk:abc 5:otc-desk:abc cred-7"]);
    expect(wallet.seed).toEqual(new Uint8Array(32).fill(5));
    expect(credentials.data).toEqual({ "u-1": "cred-7" });
  });

  it("names a passkey the desk does not know", async () => {
    const a = api();
    a.api.createChallenge = async () => {
      throw Object.assign(new Error("not found"), { status: 404 });
    };
    const flows = createAuthFlows({ auth: a.api, vault: vault().vault, keys: keys(), credentials: store() });

    await expect(flows.connectWallet()).rejects.toBeInstanceOf(UnknownPasskeyError);
  });

  it("refuses a blob that opens to another key, without signing in", async () => {
    const a = api({ publicKey: enc.encode("pk6") });
    const flows = createAuthFlows({ auth: a.api, vault: vault().vault, keys: keys(), credentials: store() });

    await expect(flows.connectWallet()).rejects.toBeInstanceOf(KeyMismatchError);
    expect(a.log).toEqual(["challenge cred-found"]);
  });
});

describe("unlock", () => {
  it("signs in again with the user's own passkey, so the session belongs to the unlocked key", async () => {
    const v = vault();
    const a = api();
    const credentials = store({ "u-1": "cred-7" });
    const flows = createAuthFlows({ auth: a.api, vault: v.vault, keys: keys(), credentials });

    const wallet = await flows.unlock(ME);

    expect(v.log).toEqual(["unlock cred-7"]);
    expect(a.log).toEqual(["challenge cred-7", "login otc-desk:abc 5:otc-desk:abc cred-7"]);
    expect(wallet).toEqual({ me: ME, seed: new Uint8Array(32).fill(5) });
    expect(credentials.data).toEqual({ "u-1": "cred-7" });
  });

  it("lets the platform pick the passkey when this browser never saw one", async () => {
    const v = vault({ discovered: "cred-9" });
    const credentials = store();
    const flows = createAuthFlows({ auth: api().api, vault: v.vault, keys: keys(), credentials });

    await flows.unlock(ME);

    expect(v.log).toEqual(["unlock (any)"]);
    expect(credentials.data).toEqual({ "u-1": "cred-9" });
  });

  it("with another wallet's passkey, signs in as that wallet and remembers only that", async () => {
    const credentials = store();
    const flows = createAuthFlows({
      auth: api().api,
      vault: vault({ discovered: "cred-bob" }).vault,
      keys: keys(),
      credentials,
    });

    const wallet = await flows.unlock(ME);

    expect(wallet.me).toEqual(BOB);
    expect(credentials.data).toEqual({ "u-2": "cred-bob" });
  });

  it("remembers no passkey when the login fails", async () => {
    const a = api();
    a.api.login = async () => {
      throw Object.assign(new Error("bad signature"), { status: 401 });
    };
    const credentials = store();
    const flows = createAuthFlows({ auth: a.api, vault: vault({ discovered: "cred-9" }).vault, keys: keys(), credentials });

    await expect(flows.unlock(ME)).rejects.toThrow("bad signature");
    expect(credentials.data).toEqual({});
  });
});

describe("logout", () => {
  it("ends the session on the server", async () => {
    const a = api();
    const flows = createAuthFlows({ auth: a.api, vault: vault().vault, keys: keys(), credentials: store() });

    await flows.logout();

    expect(a.log).toEqual(["logout"]);
  });
});
