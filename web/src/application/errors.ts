export interface ApiFailure extends Error {
  readonly status: number;
  readonly code?: string;
}

export function isApiFailure(e: unknown): e is ApiFailure {
  return e instanceof Error && typeof (e as { status?: unknown }).status === "number";
}

export class PrfUnsupportedError extends Error {
  constructor() {
    super(
      "This passkey can't protect a wallet key: the browser or authenticator does not support the WebAuthn PRF extension. Use a current Chrome, Edge or Safari with a synced passkey (iCloud Keychain, Google Password Manager, Windows Hello).",
    );
    this.name = "PrfUnsupportedError";
  }
}

export class PasskeyCancelledError extends Error {
  constructor(what: string) {
    super(`${what} was cancelled`);
    this.name = "PasskeyCancelledError";
  }
}

export class UnknownPasskeyError extends Error {
  readonly credentialId: string;

  constructor(credentialId: string) {
    super("No wallet is registered for this passkey on this desk — create one instead.");
    this.name = "UnknownPasskeyError";
    this.credentialId = credentialId;
  }
}

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
