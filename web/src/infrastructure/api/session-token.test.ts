import { describe, expect, it } from "vitest";
import { createSessionTokens, type TokenStorage } from "./session-token";

const NOW = Date.parse("2026-10-24T12:00:00Z");
const LATER = "2026-10-31T12:00:00Z";

function memoryStorage(): TokenStorage & { data: Map<string, string> } {
  const data = new Map<string, string>();
  return {
    data,
    getItem: (k) => data.get(k) ?? null,
    setItem: (k, v) => void data.set(k, v),
    removeItem: (k) => void data.delete(k),
  };
}

const refusing: TokenStorage = {
  getItem: () => {
    throw new DOMException("denied", "SecurityError");
  },
  setItem: () => {
    throw new DOMException("denied", "SecurityError");
  },
  removeItem: () => {
    throw new DOMException("denied", "SecurityError");
  },
};

describe("createSessionTokens", () => {
  it("has no token at first", () => {
    expect(createSessionTokens(memoryStorage(), () => NOW).get()).toBeUndefined();
  });

  it("keeps the token under otc-desk:session, so a reload of the tab still has it", () => {
    const storage = memoryStorage();
    createSessionTokens(storage, () => NOW).set("tok-1", LATER);

    expect([...storage.data.keys()]).toEqual(["otc-desk:session"]);
    expect(createSessionTokens(storage, () => NOW).get()).toBe("tok-1");
  });

  it("forgets the token on clear", () => {
    const storage = memoryStorage();
    const tokens = createSessionTokens(storage, () => NOW);
    tokens.set("tok-1", LATER);
    expect(tokens.get()).toBe("tok-1");

    tokens.clear();

    expect(tokens.get()).toBeUndefined();
    expect(storage.data.size).toBe(0);
  });

  it("treats an expired token as none and drops it", () => {
    const storage = memoryStorage();
    let now = NOW;
    const tokens = createSessionTokens(storage, () => now);
    tokens.set("tok-1", "2026-10-24T12:30:00Z");
    expect(tokens.get()).toBe("tok-1");

    now = Date.parse("2026-10-24T12:30:00Z");

    expect(tokens.get()).toBeUndefined();
    expect(storage.data.size).toBe(0);
  });

  it("ignores what it cannot read back", () => {
    const storage = memoryStorage();
    storage.data.set("otc-desk:session", "{not json");
    expect(createSessionTokens(storage, () => NOW).get()).toBeUndefined();
    storage.data.set("otc-desk:session", JSON.stringify({ token: 5, expiresAt: "soon" }));
    expect(createSessionTokens(storage, () => NOW).get()).toBeUndefined();
  });

  it("still works, for this page, when storage is refused", () => {
    const tokens = createSessionTokens(refusing, () => NOW);
    tokens.set("tok-1", LATER);
    expect(tokens.get()).toBe("tok-1");
    tokens.clear();
    expect(tokens.get()).toBeUndefined();

    const none = createSessionTokens(undefined, () => NOW);
    none.set("tok-2", LATER);
    expect(none.get()).toBe("tok-2");
  });
});
