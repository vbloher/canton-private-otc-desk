/**
 * Ed25519 party keys (@noble/ed25519, whose async API hashes with WebCrypto's
 * SHA-512). The 32-byte seed is the private key; its raw public key registers
 * the external party, and every signature the ledger sees (onboarding hash,
 * login challenge, prepared transactions) comes from here.
 */
import { getPublicKeyAsync, signAsync } from "@noble/ed25519";
import type { PartyKeys } from "@/application/ports";

export const ed25519Keys: PartyKeys = {
  generateSeed: () => crypto.getRandomValues(new Uint8Array(32)),
  publicKey: (seed) => getPublicKeyAsync(seed),
  signer: (seed) => ({ sign: (message) => signAsync(message, seed) }),
};
