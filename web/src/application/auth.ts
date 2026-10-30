import type { Me } from "@/domain/types";
import { isApiFailure, KeyMismatchError, UnknownPasskeyError } from "./errors";
import type { AuthApi, CredentialStore, KeyVault, LoginChallenge, PartyKeys, UnlockedPasskey } from "./ports";

export interface AuthDeps {
  auth: AuthApi;
  vault: KeyVault;
  keys: PartyKeys;
  credentials: CredentialStore;
}

export type CreateStep = "party" | "passkey" | "unlock";

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

interface PendingWallet {
  name: string;
  seed: Uint8Array;
  registrationId: string;
  signature: Uint8Array;
  credentialId?: string;
}

const equalBytes = (a: Uint8Array, b: Uint8Array): boolean =>
  a.length === b.length && a.every((v, i) => v === b[i]);

export function createAuthFlows(deps: AuthDeps): AuthFlows {
  let pending: PendingWallet | undefined;

  async function partyFor(name: string): Promise<PendingWallet> {
    if (pending?.name === name) return pending;
    pending?.seed.fill(0);
    pending = undefined;
    const seed = deps.keys.generateSeed();
    try {
      const registration = await deps.auth.registerStart({
        displayName: name,
        publicKey: await deps.keys.publicKey(seed),
      });
      const signature = await deps.keys.signer(seed).sign(registration.multiHash);
      await deps.auth.registerParty({ registrationId: registration.registrationId, signature });
      pending = { name, seed, registrationId: registration.registrationId, signature };
      return pending;
    } catch (e) {
      seed.fill(0);
      throw e;
    }
  }

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
      onStep?.("party");
      const wallet = await partyFor(name);
      onStep?.("passkey");
      wallet.credentialId ??= await deps.vault.createPasskey(name);
      onStep?.("unlock");
      const passkey = await deps.vault.unlock(wallet.credentialId);
      let me: Me;
      try {
        me = await deps.auth.registerComplete({
          registrationId: wallet.registrationId,
          signature: wallet.signature,
          credentialId: passkey.credentialId,
          encryptedBlob: await passkey.wrap(wallet.seed),
        });
      } catch (e) {
        if (isApiFailure(e) && e.status === 410) {
          wallet.seed.fill(0);
          pending = undefined;
        }
        throw e;
      }
      pending = undefined;
      deps.credentials.set(me.id, passkey.credentialId);
      return { me, seed: wallet.seed };
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
