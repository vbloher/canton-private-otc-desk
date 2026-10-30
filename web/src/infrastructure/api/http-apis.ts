import createClient from "openapi-fetch";
import { isApiFailure } from "@/application/errors";
import type { AuthApi, DeskApi, WalletApi } from "@/application/ports";
import { toBase64 } from "../encoding";
import { apiErrorFrom, isNetworkFailure, unreachable } from "./api-error";
import {
  toBalance,
  toChallenge,
  toIncomingRfq,
  toIncomingTransfer,
  toInstrument,
  toMaker,
  toMe,
  toNetwork,
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
  baseUrl: string;
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

async function done(pending: Promise<Answer<unknown>>): Promise<void> {
  await unwrap(pending);
}

export function createHttpApis(options: HttpApiOptions): HttpApis {
  const init = { baseUrl: options.baseUrl, ...(options.fetch === undefined ? {} : { fetch: options.fetch }) };
  const identity = createClient<IdentityPaths>(init);
  const trading = createClient<TradingPaths>(init);
  const wallet = createClient<WalletPaths>(init);

  const auth: AuthApi = {
    registerStart: async ({ displayName, publicKey }) =>
      toRegistration(
        await unwrap(identity.POST("/auth/register/start", { body: { displayName, publicKey: toBase64(publicKey) } })),
      ),
    registerParty: async ({ registrationId, signature }) =>
      (
        await unwrap(
          identity.POST("/auth/register/party", { body: { registrationId, signature: toBase64(signature) } }),
        )
      ).partyId,
    registerComplete: async ({ registrationId, signature, credentialId, encryptedBlob }) =>
      toMe(
        await unwrap(
          identity.POST("/auth/register/complete", {
            body: { registrationId, signature: toBase64(signature), credentialId, encryptedBlob },
          }),
        ),
      ),
    createChallenge: async (credentialId) =>
      toChallenge(await unwrap(identity.POST("/auth/challenge", { body: { credentialId } }))),
    login: async ({ challenge, signature, credentialId }) =>
      toMe(
        await unwrap(identity.POST("/auth/login", { body: { challenge, signature: toBase64(signature), credentialId } })),
      ),
    logout: () => done(identity.POST("/auth/logout", {})),
    me: async () => {
      try {
        return toMe(await unwrap(identity.GET("/me")));
      } catch (e) {
        if (isApiFailure(e) && e.status === 401) return null;
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
    network: async () => toNetwork(await unwrap(wallet.GET("/wallet/network"))),
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
