/**
 * Browser identity flows. The wallet is an external Canton party whose Ed25519
 * seed is generated here and wrapped with a passkey's PRF output; the server
 * only ever stores the wrapped blob.
 *
 * - createWallet: passkey → PRF unlock (the PRF secret only comes with get())
 *   → fresh seed → register/start → sign the multi-hash → register/complete.
 * - connectWallet: PRF unlock of any discoverable passkey → challenge → unwrap
 *   → sign the challenge's UTF-8 bytes → login.
 * - unlock: after a reload the session survives (the cookie, or the token the
 *   tab keeps when the gateway is on another origin) but the seed does not.
 *   It runs the connect flow again, offering the passkey this browser last
 *   used for the user. Signing in again guarantees the session belongs to the
 *   key just unlocked: a passkey of another wallet signs in as that wallet,
 *   and the caller sees the identity change.
 */
import type { Me } from "@/domain/types";
import { isApiFailure, KeyMismatchError, UnknownPasskeyError } from "./errors";
import type { AuthApi, CredentialStore, KeyVault, LoginChallenge, PartyKeys, UnlockedPasskey } from "./ports";

export interface AuthDeps {
  auth: AuthApi;
  vault: KeyVault;
  keys: PartyKeys;
  credentials: CredentialStore;
}

/** Progress of createWallet: the two passkey prompts, then the party allocation. */
export type CreateStep = "passkey" | "unlock" | "party";

/** A signed-in user with the party seed unlocked in this tab. */
export interface Wallet {
  me: Me;
  seed: Uint8Array;
}

export interface AuthFlows {
  createWallet(displayName: string, onStep?: (step: CreateStep) => void): Promise<Wallet>;
  connectWallet(): Promise<Wallet>;
  /** The wallet whose key the passkey opened — normally `me`'s, but check `me.id`. */
  unlock(me: Me): Promise<Wallet>;
  logout(): Promise<void>;
}

const equalBytes = (a: Uint8Array, b: Uint8Array): boolean =>
  a.length === b.length && a.every((v, i) => v === b[i]);

export function createAuthFlows(deps: AuthDeps): AuthFlows {
  /**
   * A passkey created by a sign-up that failed later (a cancelled second prompt,
   * a ledger error). Nothing was registered, so a retry under the same name
   * reuses it instead of leaving another orphan in the authenticator.
   */
  let orphan: { name: string; credentialId: string } | undefined;

  async function challengeFor(passkey: UnlockedPasskey): Promise<LoginChallenge> {
    try {
      return await deps.auth.createChallenge(passkey.credentialId);
    } catch (e) {
      if (isApiFailure(e) && e.status === 404) throw new UnknownPasskeyError(passkey.credentialId);
      throw e;
    }
  }

  /** Unwrap the blob and make sure it is the key the server knows for this passkey. */
  async function openSeed(passkey: UnlockedPasskey, challenge: LoginChallenge): Promise<Uint8Array> {
    const seed = await passkey.unwrap(challenge.encryptedBlob);
    if (!equalBytes(await deps.keys.publicKey(seed), challenge.publicKey)) {
      seed.fill(0);
      throw new KeyMismatchError();
    }
    return seed;
  }

  /** Challenge → unwrap → sign → login; the passkey is remembered only once the login succeeded. */
  async function signInWith(passkey: UnlockedPasskey): Promise<Wallet> {
    const challenge = await challengeFor(passkey);
    const seed = await openSeed(passkey, challenge);
    try {
      const me = await deps.auth.login({
        challenge: challenge.challenge,
        signature: await deps.keys.signer(seed).sign(new TextEncoder().encode(challenge.challenge)),
        credentialId: passkey.credentialId,
      });
      deps.credentials.set(me.id, passkey.credentialId);
      return { me, seed };
    } catch (e) {
      seed.fill(0);
      throw e;
    }
  }

  return {
    async createWallet(displayName, onStep) {
      const name = displayName.trim();
      onStep?.("passkey");
      const credentialId =
        orphan?.name === name ? orphan.credentialId : await deps.vault.createPasskey(name);
      orphan = { name, credentialId };
      onStep?.("unlock");
      const passkey = await deps.vault.unlock(credentialId);
      onStep?.("party");
      const seed = deps.keys.generateSeed();
      const registration = await deps.auth.registerStart({
        displayName: name,
        publicKey: await deps.keys.publicKey(seed),
      });
      const me = await deps.auth.registerComplete({
        registrationId: registration.registrationId,
        signature: await deps.keys.signer(seed).sign(registration.multiHash),
        credentialId: passkey.credentialId,
        encryptedBlob: await passkey.wrap(seed),
      });
      orphan = undefined;
      deps.credentials.set(me.id, passkey.credentialId);
      return { me, seed };
    },

    async connectWallet() {
      return signInWith(await deps.vault.unlock());
    },

    async unlock(me) {
      return signInWith(await deps.vault.unlock(deps.credentials.get(me.id)));
    },

    async logout() {
      await deps.auth.logout();
    },
  };
}
