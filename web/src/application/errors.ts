/**
 * Failures the application reasons about. Infrastructure throws them; the
 * presentation tells them apart without knowing where they came from.
 */

/** A desk API call that answered with an error: HTTP status, `error` text, optional `code`. */
export interface ApiFailure extends Error {
  readonly status: number;
  readonly code?: string;
}

export function isApiFailure(e: unknown): e is ApiFailure {
  return e instanceof Error && typeof (e as { status?: unknown }).status === "number";
}

/** The passkey works but cannot produce a PRF secret, so it cannot protect a wallet key. */
export class PrfUnsupportedError extends Error {
  constructor() {
    super(
      "This passkey can't protect a wallet key: the browser or authenticator does not support the WebAuthn PRF extension. Use a current Chrome, Edge or Safari with a synced passkey (iCloud Keychain, Google Password Manager, Windows Hello).",
    );
    this.name = "PrfUnsupportedError";
  }
}

/** The user dismissed the passkey prompt (or the platform refused it). */
export class PasskeyCancelledError extends Error {
  constructor(what: string) {
    super(`${what} was cancelled`);
    this.name = "PasskeyCancelledError";
  }
}

/** A real passkey with no wallet behind it on this desk. */
export class UnknownPasskeyError extends Error {
  readonly credentialId: string;

  constructor(credentialId: string) {
    super("No wallet is registered for this passkey on this desk — create one instead.");
    this.name = "UnknownPasskeyError";
    this.credentialId = credentialId;
  }
}

/** The blob opened to a key that is not the wallet's public key. */
export class KeyMismatchError extends Error {
  constructor() {
    super("This passkey unlocked a key that does not belong to this wallet.");
    this.name = "KeyMismatchError";
  }
}

export function errorMessage(e: unknown): string {
  if (e instanceof Error && e.message !== "") return e.message;
  if (typeof e === "string" && e !== "") return e;
  return "Something went wrong";
}
