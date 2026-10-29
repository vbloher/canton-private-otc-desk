import { PasskeyCancelledError, PrfUnsupportedError } from "@/application/errors";
import type { KeyVault, PasskeySupport, UnlockedPasskey } from "@/application/ports";
import { fromBase64Url, toBase64Url } from "../encoding";
import { unwrapSeed, wrapSeed } from "./blob";

export const RP_NAME = "Private OTC Desk";
export const PRF_SALT = "otc-desk-prf-salt-v1";
const TIMEOUT_MS = 120_000;

export interface WebAuthnEnv {
  rpId: string;
  credentials: Pick<CredentialsContainer, "create" | "get">;
  capabilities?: () => Promise<Record<string, boolean> | undefined>;
}

export function browserWebAuthn(): WebAuthnEnv | undefined {
  if (typeof window === "undefined" || !window.isSecureContext) return undefined;
  if (typeof PublicKeyCredential === "undefined" || navigator.credentials === undefined) return undefined;
  const pkc = PublicKeyCredential as unknown as {
    getClientCapabilities?: () => Promise<Record<string, boolean>>;
  };
  return {
    rpId: window.location.hostname,
    credentials: navigator.credentials,
    capabilities: async () => (pkc.getClientCapabilities === undefined ? undefined : pkc.getClientCapabilities()),
  };
}

const random = (n: number): Uint8Array<ArrayBuffer> => crypto.getRandomValues(new Uint8Array(n));

function asBytes(source: BufferSource): Uint8Array {
  return ArrayBuffer.isView(source)
    ? new Uint8Array(source.buffer, source.byteOffset, source.byteLength).slice()
    : new Uint8Array(source).slice();
}

async function ceremony<T>(what: string, run: () => Promise<T>): Promise<T> {
  try {
    return await run();
  } catch (e) {
    if (e instanceof DOMException) {
      if (e.name === "NotAllowedError" || e.name === "AbortError") throw new PasskeyCancelledError(what);
      if (e.name === "SecurityError") throw new Error("Passkeys need a secure page (https or localhost) on this host.", { cause: e });
      if (e.name === "NotSupportedError") throw new PrfUnsupportedError();
    }
    throw e;
  }
}

export function createPasskeyVault(env: WebAuthnEnv | undefined): KeyVault {
  function need(): WebAuthnEnv {
    if (env === undefined) throw new Error("This browser cannot use passkeys here — open the desk over https or on localhost.");
    return env;
  }

  return {
    async support(): Promise<PasskeySupport> {
      if (env === undefined) return "none";
      const caps = await env.capabilities?.().catch(() => undefined);
      const prf = caps?.["extension:prf"];
      return prf === true ? "supported" : prf === false ? "no-prf" : "unknown";
    },

    async createPasskey(displayName) {
      const { credentials, rpId } = need();
      const credential = await ceremony("Passkey creation", () =>
        credentials.create({
          publicKey: {
            challenge: random(32),
            rp: { name: RP_NAME, id: rpId },
            user: { id: random(16), name: displayName, displayName },
            pubKeyCredParams: [
              { type: "public-key", alg: -8 },
              { type: "public-key", alg: -7 },
              { type: "public-key", alg: -257 },
            ],
            authenticatorSelection: { residentKey: "required", requireResidentKey: true, userVerification: "required" },
            extensions: { prf: {} },
            timeout: TIMEOUT_MS,
          },
        }),
      );
      if (credential === null) throw new PasskeyCancelledError("Passkey creation");
      return toBase64Url(new Uint8Array((credential as PublicKeyCredential).rawId));
    },

    async unlock(credentialId): Promise<UnlockedPasskey> {
      const { credentials, rpId } = need();
      const assertion = await ceremony("Passkey sign-in", () =>
        credentials.get({
          publicKey: {
            challenge: random(32),
            rpId,
            allowCredentials:
              credentialId === undefined ? [] : [{ type: "public-key", id: fromBase64Url(credentialId) }],
            userVerification: "required",
            extensions: { prf: { eval: { first: new TextEncoder().encode(PRF_SALT) } } },
            timeout: TIMEOUT_MS,
          },
        }),
      );
      if (assertion === null) throw new PasskeyCancelledError("Passkey sign-in");
      const pkc = assertion as PublicKeyCredential;
      const first = pkc.getClientExtensionResults().prf?.results?.first;
      if (first === undefined) throw new PrfUnsupportedError();
      const secret = asBytes(first);
      return {
        credentialId: toBase64Url(new Uint8Array(pkc.rawId)),
        wrap: (seed) => wrapSeed(seed, secret),
        unwrap: (blob) => unwrapSeed(blob, secret),
      };
    },
  };
}
