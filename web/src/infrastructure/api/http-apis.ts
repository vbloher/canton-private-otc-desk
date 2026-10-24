/**
 * The application's API ports over HTTP: openapi-fetch clients typed by the
 * generated schemas (pnpm gen:api), one per contract, all on the gateway's
 * /api/v1 — the page's own origin, or the one VITE_API_URL names.
 *
 * The session travels one of two ways (config.sessionTransport). On the page's
 * own origin it is the HttpOnly cookie alone: no token is kept or sent. With a
 * gateway on another origin, requests carry no credentials (so the gateway
 * needs no Allow-Credentials); the token sign-in answers with is kept and sent
 * as `Authorization: Bearer …`.
 */
import createClient, { type Middleware } from "openapi-fetch";
import { isApiFailure } from "@/application/errors";
import type { AuthApi, DeskApi, WalletApi } from "@/application/ports";
import { toBase64 } from "../encoding";
import { apiErrorFrom, isNetworkFailure, unreachable } from "./api-error";
import type { components as IdentitySchema } from "./schema/identity";
import type { SessionTokens } from "./session-token";
import {
  toBalance,
  toChallenge,
  toIncomingRfq,
  toIncomingTransfer,
  toInstrument,
  toMaker,
  toMe,
  toQuote,
  toRegistration,
  toRfq,
  toRfqWithQuotes,
  toSignAction,
  toTrade,
} from "./mappers";
import type { paths as IdentityPaths } from "./schema/identity";
import type { paths as TradingPaths } from "./schema/trading";
import type { paths as WalletPaths } from "./schema/wallet";

export interface HttpApiOptions {
  /** Absolute base, e.g. `${location.origin}/api/v1` (config.apiBaseUrl). */
  baseUrl: string;
  /** Bearer mode (gateway on another origin): where the token from sign-in is kept. Omitted: cookie mode. */
  tokens?: SessionTokens;
  fetch?: (input: Request) => Promise<Response>;
}

export interface HttpApis {
  auth: AuthApi;
  desk: DeskApi;
  wallet: WalletApi;
}

interface Answer<D> {
  data?: D;
  error?: unknown;
  response: Response;
}

/** The data of a 2xx answer; an error answer becomes an ApiError, a lost request an unreachable one. */
async function unwrap<D>(pending: Promise<Answer<D>>): Promise<D> {
  let answer: Answer<D>;
  try {
    answer = await pending;
  } catch (e) {
    throw isNetworkFailure(e) ? unreachable(e) : e;
  }
  if (!answer.response.ok) throw apiErrorFrom(answer.response.status, answer.error);
  return answer.data as D;
}

/** For 204 answers. */
async function done(pending: Promise<Answer<unknown>>): Promise<void> {
  await unwrap(pending);
}

export function createHttpApis(options: HttpApiOptions): HttpApis {
  const { tokens } = options;
  const init = {
    baseUrl: options.baseUrl,
    credentials: "same-origin" as const,
    ...(options.fetch === undefined ? {} : { fetch: options.fetch }),
  };
  const identity = createClient<IdentityPaths>(init);
  const trading = createClient<TradingPaths>(init);
  const wallet = createClient<WalletPaths>(init);
  if (tokens !== undefined) {
    const bearer: Middleware = {
      onRequest({ request }) {
        const token = tokens.get();
        // A request may name its token itself (me() does); otherwise it carries the current one.
        if (token !== undefined && !request.headers.has("Authorization")) {
          request.headers.set("Authorization", `Bearer ${token}`);
        }
        return request;
      },
    };
    for (const client of [identity, trading, wallet]) client.use(bearer);
  }

  /** Sign-in answers with the session: keep its token in bearer mode, hand on the user. */
  const signedIn = (session: IdentitySchema["schemas"]["Session"]) => {
    tokens?.set(session.sessionToken, session.expiresAt);
    return toMe(session.me);
  };

  const auth: AuthApi = {
    registerStart: async ({ displayName, publicKey }) =>
      toRegistration(
        await unwrap(identity.POST("/auth/register/start", { body: { displayName, publicKey: toBase64(publicKey) } })),
      ),
    registerComplete: async ({ registrationId, signature, credentialId, encryptedBlob }) =>
      signedIn(
        await unwrap(
          identity.POST("/auth/register/complete", {
            body: { registrationId, signature: toBase64(signature), credentialId, encryptedBlob },
          }),
        ),
      ),
    createChallenge: async (credentialId) =>
      toChallenge(await unwrap(identity.POST("/auth/challenge", { body: { credentialId } }))),
    login: async ({ challenge, signature, credentialId }) =>
      signedIn(
        await unwrap(identity.POST("/auth/login", { body: { challenge, signature: toBase64(signature), credentialId } })),
      ),
    logout: async () => {
      try {
        await done(identity.POST("/auth/logout", {}));
      } finally {
        tokens?.clear(); // signed out here whatever the desk answered
      }
    },
    me: async () => {
      // The token this request carries, named explicitly, so its 401 can be told apart from a newer session.
      const sent = tokens?.get();
      try {
        return toMe(
          await unwrap(identity.GET("/me", sent === undefined ? {} : { headers: { Authorization: `Bearer ${sent}` } })),
        );
      } catch (e) {
        if (isApiFailure(e) && e.status === 401) {
          // That session is over. A login that finished meanwhile stored a newer token: it stays.
          if (tokens !== undefined && tokens.get() === sent) tokens.clear();
          return null;
        }
        throw e;
      }
    },
    setMakerMode: async (enabled) => toMe(await unwrap(identity.PUT("/me/maker-mode", { body: { enabled } }))),
    makers: async () => (await unwrap(identity.GET("/makers"))).map(toMaker),
  };

  const desk: DeskApi = {
    desk: async () => ({ feeBps: (await unwrap(trading.GET("/desk"))).feeBps }),
    createRfq: async (input) =>
      toRfq(
        await unwrap(
          trading.POST("/rfqs", {
            body: {
              direction: input.direction,
              qty: input.qty,
              base: input.base,
              quote: input.quote,
              makers: input.makers,
              ttlSeconds: input.ttlSeconds,
            },
          }),
        ),
      ),
    myRfqs: async () => (await unwrap(trading.GET("/rfqs/mine"))).map(toRfqWithQuotes),
    incomingRfqs: async () => (await unwrap(trading.GET("/rfqs/incoming"))).map(toIncomingRfq),
    rfqHistory: async () => (await unwrap(trading.GET("/rfqs/history"))).map(toRfq),
    cancelRfq: (id) => done(trading.DELETE("/rfqs/{id}", { params: { path: { id } } })),
    submitQuote: async (id, input) =>
      toQuote(
        await unwrap(
          trading.POST("/rfqs/{id}/quotes", {
            params: { path: { id } },
            body: input.ttlSeconds === undefined ? { price: input.price } : { price: input.price, ttlSeconds: input.ttlSeconds },
          }),
        ),
      ),
    acceptQuote: async (id) => toTrade(await unwrap(trading.POST("/quotes/{id}/accept", { params: { path: { id } } }))),
    trades: async () => (await unwrap(trading.GET("/trades"))).map(toTrade),
  };

  const walletApi: WalletApi = {
    instruments: async () => (await unwrap(wallet.GET("/instruments"))).map(toInstrument),
    balances: async () => (await unwrap(wallet.GET("/wallet/balances"))).map(toBalance),
    incoming: async () => (await unwrap(wallet.GET("/wallet/incoming"))).map(toIncomingTransfer),
    acceptIncoming: async (contractId) =>
      (
        await unwrap(wallet.POST("/wallet/incoming/{contractId}/accept", { params: { path: { contractId } } }))
      ).actions.map(toSignAction),
    withdraw: async ({ symbol, amount, receiver }) =>
      (await unwrap(wallet.POST("/wallet/withdraw", { body: { symbol, amount, receiver } }))).actions.map(toSignAction),
    faucet: async () => (await unwrap(wallet.POST("/wallet/faucet"))).actions.map(toSignAction),
    pendingActions: async () => (await unwrap(wallet.GET("/tx/pending"))).map(toSignAction),
    executeAction: (id, signature) =>
      done(wallet.POST("/tx/{id}/execute", { params: { path: { id } }, body: { signature: toBase64(signature) } })),
  };

  return { auth, desk, wallet: walletApi };
}
