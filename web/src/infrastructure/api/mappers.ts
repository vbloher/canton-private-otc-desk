/**
 * Wire DTOs (generated from api/http/*.yaml) → domain entities: RFC 3339
 * timestamps become epoch milliseconds and base64 byte fields become bytes.
 */
import type { Registration, LoginChallenge } from "@/application/ports";
import type {
  Balance,
  IncomingRfq,
  IncomingTransfer,
  Instrument,
  Maker,
  Me,
  Quote,
  Rfq,
  RfqWithQuotes,
  SignAction,
  Trade,
} from "@/domain/types";
import { fromBase64 } from "../encoding";
import type { components as IdentitySchema } from "./schema/identity";
import type { components as TradingSchema } from "./schema/trading";
import type { components as WalletSchema } from "./schema/wallet";

type IdentityDto = IdentitySchema["schemas"];
type TradingDto = TradingSchema["schemas"];
type WalletDto = WalletSchema["schemas"];

/** Go writes nanoseconds; Date.parse is only specified for milliseconds. */
export function parseTime(iso: string): number {
  const ms = Date.parse(iso.replace(/(\.\d{3})\d+/, "$1"));
  if (Number.isNaN(ms)) throw new Error(`The desk sent an unreadable time: ${iso}`);
  return ms;
}

export const toMe = (d: IdentityDto["Me"]): Me => ({
  id: d.id,
  partyId: d.partyId,
  hint: d.hint,
  displayName: d.displayName,
  kind: d.kind,
  makerMode: d.makerMode,
});

export const toMaker = (d: IdentityDto["Maker"]): Maker => ({ hint: d.hint, displayName: d.displayName });

export const toRegistration = (d: IdentityDto["RegisterStartOutput"]): Registration => ({
  registrationId: d.registrationId,
  partyId: d.partyId,
  multiHash: fromBase64(d.multiHash),
});

export const toChallenge = (d: IdentityDto["ChallengeOutput"]): LoginChallenge => ({
  challenge: d.challenge,
  encryptedBlob: d.encryptedBlob,
  publicKey: fromBase64(d.publicKey),
});

export const toRfq = (d: TradingDto["Rfq"]): Rfq => ({
  id: d.id,
  taker: d.taker,
  direction: d.direction,
  qty: d.qty,
  base: d.base,
  quote: d.quote,
  makers: [...d.makers],
  feeBps: d.feeBps,
  deadline: parseTime(d.deadline),
  status: d.status,
  createdAt: parseTime(d.createdAt),
});

export const toQuote = (d: TradingDto["Quote"]): Quote => ({
  id: d.id,
  rfqId: d.rfqId,
  maker: d.maker,
  price: d.price,
  validUntil: parseTime(d.validUntil),
  status: d.status,
  createdAt: parseTime(d.createdAt),
});

export const toRfqWithQuotes = (d: TradingDto["RfqWithQuotes"]): RfqWithQuotes => ({
  rfq: toRfq(d.rfq),
  quotes: d.quotes.map(toQuote),
});

export function toIncomingRfq(d: TradingDto["IncomingRfq"]): IncomingRfq {
  return d.myQuote === undefined ? { rfq: toRfq(d.rfq) } : { rfq: toRfq(d.rfq), myQuote: toQuote(d.myQuote) };
}

export function toTrade(d: TradingDto["Trade"]): Trade {
  const trade: Trade = {
    id: d.id,
    rfqId: d.rfqId,
    quoteId: d.quoteId,
    taker: d.taker,
    maker: d.maker,
    direction: d.direction,
    base: d.base,
    quote: d.quote,
    qty: d.qty,
    price: d.price,
    notional: d.notional,
    fee: d.fee,
    status: d.status,
    createdAt: parseTime(d.createdAt),
  };
  if (d.reason !== undefined && d.reason !== "") trade.reason = d.reason;
  if (d.updateId !== undefined && d.updateId !== "") trade.updateId = d.updateId;
  if (d.settledAt !== undefined) trade.settledAt = parseTime(d.settledAt);
  return trade;
}

export const toInstrument = (d: WalletDto["Instrument"]): Instrument => ({
  symbol: d.symbol,
  name: d.name,
  decimals: d.decimals,
});

export const toBalance = (d: WalletDto["Balance"]): Balance => ({
  symbol: d.symbol,
  available: d.available,
  locked: d.locked,
});

export const toIncomingTransfer = (d: WalletDto["IncomingTransfer"]): IncomingTransfer => ({
  contractId: d.contractId,
  sender: d.sender,
  symbol: d.symbol,
  amount: d.amount,
});

export const toSignAction = (d: WalletDto["SignAction"]): SignAction => ({
  id: d.id,
  purpose: d.purpose,
  description: d.description,
  hash: fromBase64(d.hash),
  createdAt: parseTime(d.createdAt),
  expiresAt: parseTime(d.expiresAt),
});
