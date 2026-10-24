/**
 * The session token for a frontend on another origin than the gateway (GitHub
 * Pages): there the session cookie would be a third-party cookie, so sign-in's
 * token travels as `Authorization: Bearer …` and as a WebSocket subprotocol.
 * It is kept in sessionStorage (this tab, across reloads) and in memory when
 * storage is refused; an expired token counts as none.
 */

export type TokenStorage = Pick<Storage, "getItem" | "setItem" | "removeItem">;

export interface SessionTokens {
  /** The current token, if one is known and not expired. */
  get(): string | undefined;
  set(token: string, expiresAt: string): void;
  clear(): void;
}

const KEY = "otc-desk:session";

interface Stored {
  token: string;
  /** Epoch ms. */
  expiresAt: number;
}

function parse(raw: string | null | undefined): Stored | undefined {
  if (raw === null || raw === undefined) return undefined;
  try {
    const value: unknown = JSON.parse(raw);
    if (typeof value !== "object" || value === null) return undefined;
    const { token, expiresAt } = value as Record<string, unknown>;
    return typeof token === "string" && token !== "" && typeof expiresAt === "number" ? { token, expiresAt } : undefined;
  } catch {
    return undefined;
  }
}

export function createSessionTokens(storage: TokenStorage | undefined, now: () => number = Date.now): SessionTokens {
  let memory: Stored | undefined;

  function stored(): Stored | undefined {
    try {
      return parse(storage?.getItem(KEY));
    } catch {
      return undefined; // storage refused: the page's own copy is all there is
    }
  }

  function clear(): void {
    memory = undefined;
    try {
      storage?.removeItem(KEY);
    } catch {
      // nothing stored, then
    }
  }

  return {
    get() {
      const entry = memory ?? stored();
      if (entry === undefined) return undefined;
      if (entry.expiresAt <= now()) {
        clear();
        return undefined;
      }
      memory = entry;
      return entry.token;
    },
    set(token, expiresAt) {
      const at = Date.parse(expiresAt);
      memory = { token, expiresAt: Number.isFinite(at) ? at : Number.MAX_SAFE_INTEGER };
      try {
        storage?.setItem(KEY, JSON.stringify(memory));
      } catch {
        // private mode or a full quota: the token lasts as long as the page
      }
    },
    clear,
  };
}
