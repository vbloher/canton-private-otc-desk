import type { Me } from "@/domain/types";
import { isApiFailure, KeyMismatchError, UnknownPasskeyError } from "./errors";
import type { AuthApi, CredentialStore, KeyVault, LoginChallenge, PartyKeys, UnlockedPasskey } from "./ports";

export interface AuthDeps {
  auth: AuthApi;
  vault: KeyVault;
  keys: PartyKeys;
  credentials: CredentialStore;
}

export type CreateStep = "passkey" | "unlock" | "party";

export interface Wallet {
  me: Me;
  seed: Uint8Array;
}

export interface AuthFlows {
  createWallet(displayName: string, onStep?: (step: CreateStep) => void): Promise<Wallet>;
  connectWallet(): Promise<Wallet>;
  unlock(me: Me): Promise<Wallet>;
  logout(): Promise<void>;
}

const equalBytes = (a: Uint8Array, b: Uint8Array): boolean =>
  a.length === b.length && a.every((v, i) => v === b[i]);

export function createAuthFlows(deps: AuthDeps): AuthFlows {
  let orphan: { name: string; credentialId: string } | undefined;

  async function challengeFor(passkey: UnlockedPasskey): Promise<LoginChallenge> {
    try {
      return await deps.auth.createChallenge(passkey.credentialId);
    } catch (e) {
      if (isApiFailure(e) && e.status === 404) throw new UnknownPasskeyError(passkey.credentialId);
      throw e;
    }
  }

  async function openSeed(passkey: UnlockedPasskey, challenge: LoginChallenge): Promise<Uint8Array> {
    const seed = await passkey.unwrap(challenge.encryptedBlob);
    if (!equalBytes(await deps.keys.publicKey(seed), challenge.publicKey)) {
      seed.fill(0);
      throw new KeyMismatchError();
    }
    return seed;
  }

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
