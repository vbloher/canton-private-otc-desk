import { getPublicKeyAsync, signAsync } from "@noble/ed25519";
import type { PartyKeys } from "@/application/ports";

export const ed25519Keys: PartyKeys = {
  generateSeed: () => crypto.getRandomValues(new Uint8Array(32)),
  publicKey: (seed) => getPublicKeyAsync(seed),
  signer: (seed) => ({ sign: (message) => signAsync(message, seed) }),
};
