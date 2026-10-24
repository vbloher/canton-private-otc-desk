import { describe, expect, it } from "vitest";
import { createHttpApis } from "./http-apis";
import { createSessionTokens, type SessionTokens } from "./session-token";

interface Seen {
  method: string;
  path: string;
  body: unknown;
}

/** How a request presented itself: its Authorization header and its credentials mode. */
interface Presented {
  path: string;
  authorization: string | null;
  credentials: RequestCredentials;
}

const json = (status: number, body: unknown): Response =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

/**
 * A desk that answers from a route table ("POST /api/v1/rfqs") and records what it was sent.
 * With `tokens` the client runs in bearer mode (gateway on another origin); `null` is cookie mode.
 */
function desk(
  routes: Record<string, () => Response | Promise<Response>>,
  tokens: SessionTokens | null = createSessionTokens(undefined),
) {
  const seen: Seen[] = [];
  const presented: Presented[] = [];
  const fetch = async (input: Request | string | URL): Promise<Response> => {
    const req = input instanceof Request ? input : new Request(input);
    const url = new URL(req.url);
    const text = await req.text();
    seen.push({ method: req.method, path: url.pathname, body: text === "" ? undefined : JSON.parse(text) });
    presented.push({ path: url.pathname, authorization: req.headers.get("authorization"), credentials: req.credentials });
    const route = routes[`${req.method} ${url.pathname}`];
    return route === undefined ? json(404, { error: "no route" }) : route();
  };
  const apis = createHttpApis({ baseUrl: "http://desk.test/api/v1", fetch, ...(tokens === null ? {} : { tokens }) });
  return { apis, seen, presented, tokens };
}

const RFQ = {
  id: "0b8f4a6e-1c1d-4d39-9a43-2f3f1d6c6a01",
  taker: "alice-3f9a1c",
  direction: "BUY",
  qty: "0.5",
  base: "CBTC",
  quote: "USDCx",
  makers: ["makerA", "makerB"],
  feeBps: 10,
  deadline: "2026-10-21T12:05:00.123456789Z",
  status: "open",
  createdAt: "2026-10-21T12:00:00Z",
};
const QUOTE = {
  id: "5d2b6f1e-8a51-4b8e-9c0d-0e7b8a6f3c11",
  rfqId: RFQ.id,
  maker: "makerA",
  price: "100150.0",
  validUntil: "2026-10-21T12:01:00+00:00",
  status: "open",
  createdAt: "2026-10-21T12:00:01Z",
};

describe("desk api", () => {
  it("maps the taker's RFQs and quotes, timestamps to epoch ms", async () => {
    const { apis } = desk({ "GET /api/v1/rfqs/mine": () => json(200, [{ rfq: RFQ, quotes: [QUOTE] }]) });

    expect(await apis.desk.myRfqs()).toEqual([
      {
        rfq: { ...RFQ, deadline: Date.UTC(2026, 9, 21, 12, 5, 0, 123), createdAt: Date.UTC(2026, 9, 21, 12, 0, 0) },
        quotes: [
          {
            ...QUOTE,
            validUntil: Date.UTC(2026, 9, 21, 12, 1, 0),
            createdAt: Date.UTC(2026, 9, 21, 12, 0, 1),
          },
        ],
      },
    ]);
  });

  it("leaves myQuote out of an incoming RFQ the maker has not quoted", async () => {
    const { apis } = desk({ "GET /api/v1/rfqs/incoming": () => json(200, [{ rfq: RFQ }]) });

    const [incoming] = await apis.desk.incomingRfqs();

    expect(incoming).toBeDefined();
    expect(incoming?.myQuote).toBeUndefined();
  });

  it("sends the new RFQ as the API spells it", async () => {
    const { apis, seen } = desk({ "POST /api/v1/rfqs": () => json(201, RFQ) });

    await apis.desk.createRfq({
      direction: "SELL",
      qty: "1.25",
      base: "CETH",
      quote: "USDCx",
      makers: ["makerB"],
      ttlSeconds: 300,
    });

    expect(seen).toEqual([
      {
        method: "POST",
        path: "/api/v1/rfqs",
        body: { direction: "SELL", qty: "1.25", base: "CETH", quote: "USDCx", makers: ["makerB"], ttlSeconds: 300 },
      },
    ]);
  });

  it("throws the API's error text and code", async () => {
    const { apis } = desk({
      [`POST /api/v1/quotes/${QUOTE.id}/accept`]: () =>
        json(409, { error: "request failed: quote is not open", code: "QUOTE_NOT_OPEN" }),
    });

    await expect(apis.desk.acceptQuote(QUOTE.id)).rejects.toMatchObject({
      status: 409,
      code: "QUOTE_NOT_OPEN",
      message: "request failed: quote is not open",
    });
  });
});

describe("wallet api", () => {
  it("hands sign actions over with raw hash bytes and posts signatures as base64", async () => {
    const action = {
      id: "a7c1e0f2-7b5d-4a3e-8f61-3c2d1b0a9e88",
      purpose: "allocate",
      description: "Allocate 0.5 CBTC for quote 5d2b…",
      hash: "aGFzaA==",
      createdAt: "2026-10-21T12:00:00Z",
      expiresAt: "2026-10-21T12:10:00Z",
    };
    const { apis, seen } = desk({
      "GET /api/v1/tx/pending": () => json(200, [action]),
      [`POST /api/v1/tx/${action.id}/execute`]: () => new Response(null, { status: 204 }),
    });

    const [pending] = await apis.wallet.pendingActions();
    await apis.wallet.executeAction(action.id, new TextEncoder().encode("sig"));

    expect(new TextDecoder().decode(pending?.hash)).toBe("hash");
    expect(pending?.expiresAt).toBe(Date.UTC(2026, 9, 21, 12, 10));
    expect(seen.at(-1)).toEqual({ method: "POST", path: `/api/v1/tx/${action.id}/execute`, body: { signature: "c2ln" } });
  });

  it("returns the actions a ledger change queued", async () => {
    const { apis } = desk({ "POST /api/v1/wallet/faucet": () => json(202, { actions: [] }) });

    expect(await apis.wallet.faucet()).toEqual([]);
  });

  it("reports an unreachable desk as status 0, keeping the network error as the cause", async () => {
    const network = new TypeError("Failed to fetch");
    const apis = createHttpApis({
      baseUrl: "http://desk.test/api/v1",
      fetch: async () => {
        throw network;
      },
      tokens: createSessionTokens(undefined),
    });

    const error: unknown = await apis.wallet.balances().catch((e: unknown) => e);
    expect(error).toMatchObject({ status: 0 });
    expect((error as Error).message).toMatch(/reach/);
    expect((error as Error).cause).toBe(network);
  });

  it("does not call a failure that is not the network's an unreachable desk", async () => {
    const { apis } = desk({
      "GET /api/v1/wallet/balances": () =>
        new Response("{not json", { status: 200, headers: { "content-type": "application/json" } }),
    });

    const error: unknown = await apis.wallet.balances().catch((e: unknown) => e);
    expect(error).toBeInstanceOf(SyntaxError);
  });

  it("falls back to the status when the error body is not the API's", async () => {
    const { apis } = desk({
      "GET /api/v1/wallet/balances": () => new Response("<html>Bad Gateway</html>", { status: 502 }),
    });

    const error: unknown = await apis.wallet.balances().catch((e: unknown) => e);
    expect(error).toMatchObject({ status: 502 });
    expect((error as Error).message).toMatch(/502/);
  });
});

const LATER = "2099-01-01T00:00:00Z";

describe("auth api", () => {
  const ME = {
    id: "0f6a3c1e-2b4d-4e5f-8a9b-1c2d3e4f5a6b",
    partyId: "alice-3f9a1c::1220ab12",
    hint: "alice-3f9a1c",
    displayName: "Alice",
    kind: "passkey",
    makerMode: false,
  };

  it("reads no session as no user", async () => {
    const { apis } = desk({ "GET /api/v1/me": () => json(401, { error: "unauthorized" }) });

    expect(await apis.auth.me()).toBeNull();
  });

  it("reads the signed-in user", async () => {
    const { apis } = desk({ "GET /api/v1/me": () => json(200, ME) });

    expect(await apis.auth.me()).toEqual(ME);
  });

  it("encodes keys and signatures as base64 for registration", async () => {
    const { apis, seen } = desk({
      "POST /api/v1/auth/register/start": () =>
        json(200, { registrationId: "r-1", partyId: ME.partyId, multiHash: "bXVsdGk=" }),
      "POST /api/v1/auth/register/complete": () => json(200, { me: ME, sessionToken: "tok-reg", expiresAt: LATER }),
    });

    const reg = await apis.auth.registerStart({ displayName: "Alice", publicKey: new Uint8Array(32).fill(255) });
    await apis.auth.registerComplete({
      registrationId: reg.registrationId,
      signature: new TextEncoder().encode("sig"),
      credentialId: "-_8BAg",
      encryptedBlob: "blob",
    });

    expect(new TextDecoder().decode(reg.multiHash)).toBe("multi");
    expect(seen.map((s) => s.body)).toEqual([
      { displayName: "Alice", publicKey: "/".repeat(42) + "8=" },
      { registrationId: "r-1", signature: "c2ln", credentialId: "-_8BAg", encryptedBlob: "blob" },
    ]);
  });

  describe("session token", () => {
    const session = (token: string) => () => json(200, { me: ME, sessionToken: token, expiresAt: LATER });

    it("keeps the token register/complete answers with and sends it as a Bearer header from then on", async () => {
      const { apis, presented } = desk({
        "POST /api/v1/auth/register/complete": session("tok-reg"),
        "GET /api/v1/me": () => json(200, ME),
      });

      const me = await apis.auth.registerComplete({
        registrationId: "r-1",
        signature: new Uint8Array([1]),
        credentialId: "c",
        encryptedBlob: "b",
      });
      await apis.auth.me();

      expect(me).toEqual(ME);
      expect(presented.map((p) => p.authorization)).toEqual([null, "Bearer tok-reg"]);
    });

    it("takes the token a login answers with, replacing the previous one", async () => {
      const tokens = createSessionTokens(undefined);
      tokens.set("tok-old", LATER);
      const { apis, presented } = desk(
        { "POST /api/v1/auth/login": session("tok-login"), "GET /api/v1/trades": () => json(200, []) },
        tokens,
      );

      expect(await apis.auth.login({ challenge: "c", signature: new Uint8Array([1]), credentialId: "c" })).toEqual(ME);
      await apis.desk.trades();

      expect(presented.map((p) => p.authorization)).toEqual(["Bearer tok-old", "Bearer tok-login"]);
    });

    it("never asks for credentials beyond the same origin", async () => {
      const { apis, presented } = desk({ "GET /api/v1/instruments": () => json(200, []) });

      await apis.wallet.instruments();

      expect(presented).toEqual([{ path: "/api/v1/instruments", authorization: null, credentials: "same-origin" }]);
    });

    it("drops the token on logout, even when the desk fails to answer", async () => {
      const tokens = createSessionTokens(undefined);
      tokens.set("tok-1", LATER);
      const { apis } = desk({ "POST /api/v1/auth/logout": () => json(500, { error: "boom" }) }, tokens);

      await expect(apis.auth.logout()).rejects.toMatchObject({ status: 500 });

      expect(tokens.get()).toBeUndefined();
    });

    it("keeps no token and sends no Authorization header on the same origin (cookie mode)", async () => {
      const { apis, presented } = desk(
        { "POST /api/v1/auth/login": session("tok-login"), "GET /api/v1/me": () => json(200, ME) },
        null,
      );

      expect(await apis.auth.login({ challenge: "c", signature: new Uint8Array([1]), credentialId: "c" })).toEqual(ME);
      expect(await apis.auth.me()).toEqual(ME);

      expect(presented.map((p) => p.authorization)).toEqual([null, null]);
    });

    it("does not let a late /me 401 wipe the token a login stored meanwhile", async () => {
      const tokens = createSessionTokens(undefined);
      tokens.set("tok-old", LATER);
      let answerMe: (r: Response) => void = () => undefined;
      const { apis, presented } = desk(
        {
          "GET /api/v1/me": () => new Promise<Response>((resolve) => (answerMe = resolve)),
          "POST /api/v1/auth/login": session("tok-new"),
        },
        tokens,
      );

      const late = apis.auth.me(); // asked with tok-old
      await apis.auth.login({ challenge: "c", signature: new Uint8Array([1]), credentialId: "c" });
      answerMe(json(401, { error: "unauthorized" }));

      expect(await late).toBeNull();
      expect(presented[0]).toMatchObject({ path: "/api/v1/me", authorization: "Bearer tok-old" });
      expect(tokens.get()).toBe("tok-new");
    });

    it("drops the token when /me answers 401, but not when the desk is only down", async () => {
      const tokens = createSessionTokens(undefined);
      tokens.set("tok-1", LATER);
      let status = 502;
      const { apis } = desk({ "GET /api/v1/me": () => json(status, { error: "x" }) }, tokens);

      await expect(apis.auth.me()).rejects.toMatchObject({ status: 502 });
      expect(tokens.get()).toBe("tok-1");

      status = 401;
      expect(await apis.auth.me()).toBeNull();
      expect(tokens.get()).toBeUndefined();
    });
  });
});
