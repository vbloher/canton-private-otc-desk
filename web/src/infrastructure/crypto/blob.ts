import { bytes, fromBase64, toBase64 } from "../encoding";

export const KEK_SALT = "otc-desk-kek-v1";
const IV_BYTES = 12;
const TAG_BYTES = 16;

async function kekFrom(prfSecret: Uint8Array): Promise<CryptoKey> {
  const ikm = await crypto.subtle.importKey("raw", bytes(prfSecret), "HKDF", false, ["deriveKey"]);
  return crypto.subtle.deriveKey(
    { name: "HKDF", hash: "SHA-256", salt: new TextEncoder().encode(KEK_SALT), info: new Uint8Array(0) },
    ikm,
    { name: "AES-GCM", length: 256 },
    false,
    ["encrypt", "decrypt"],
  );
}

export async function wrapSeed(seed: Uint8Array, prfSecret: Uint8Array): Promise<string> {
  const kek = await kekFrom(prfSecret);
  const iv = crypto.getRandomValues(new Uint8Array(IV_BYTES));
  const sealed = new Uint8Array(await crypto.subtle.encrypt({ name: "AES-GCM", iv }, kek, bytes(seed)));
  const out = new Uint8Array(iv.length + sealed.length);
  out.set(iv);
  out.set(sealed, iv.length);
  return toBase64(out);
}

export async function unwrapSeed(blob: string, prfSecret: Uint8Array): Promise<Uint8Array> {
  const raw = fromBase64(blob);
  if (raw.length <= IV_BYTES + TAG_BYTES) throw new Error("wallet key blob is too short");
  const kek = await kekFrom(prfSecret);
  const plain = await crypto.subtle.decrypt(
    { name: "AES-GCM", iv: raw.subarray(0, IV_BYTES) },
    kek,
    raw.subarray(IV_BYTES),
  );
  return new Uint8Array(plain);
}
