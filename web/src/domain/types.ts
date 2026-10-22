/**
 * Desk entities as the application sees them. Money is an exact decimal string
 * (see money.ts), every timestamp is epoch milliseconds, and party fields are
 * party hints ("makerA", "alice-3f9a1c") except `Me.partyId`.
 */

export type Direction = "BUY" | "SELL";

export type UserKind = "passkey" | "house";

export interface Me {
  id: string;
  partyId: string;
  hint: string;
  displayName: string;
  kind: UserKind;
  makerMode: boolean;
}

export interface Maker {
  hint: string;
  displayName: string;
}

export interface DeskTerms {
  /** Desk fee in basis points of the notional, paid by the maker. */
  feeBps: number;
}

export interface Instrument {
  symbol: string;
  name: string;
  /** Fraction digits to show. */
  decimals: number;
}

export type RfqStatus = "open" | "filled" | "expired" | "cancelled";

export interface Rfq {
  id: string;
  taker: string;
  /** The taker's view: BUY = the taker buys `base`. */
  direction: Direction;
  qty: string;
  base: string;
  quote: string;
  makers: string[];
  feeBps: number;
  deadline: number;
  status: RfqStatus;
  createdAt: number;
}

export type QuoteStatus =
  | "allocating"
  | "open"
  | "accepted"
  | "won"
  | "lost"
  | "expired"
  | "cancelled"
  | "failed";

export interface Quote {
  id: string;
  rfqId: string;
  maker: string;
  /** Quote units per one base unit. */
  price: string;
  validUntil: number;
  status: QuoteStatus;
  createdAt: number;
}

/** A taker's RFQ with the quotes it received. */
export interface RfqWithQuotes {
  rfq: Rfq;
  quotes: Quote[];
}

/** An RFQ a maker was asked to quote, with that maker's quote once there is one. */
export interface IncomingRfq {
  rfq: Rfq;
  myQuote?: Quote;
}

export type TradeStatus = "settling" | "settled" | "failed";

export interface Trade {
  id: string;
  rfqId: string;
  quoteId: string;
  taker: string;
  maker: string;
  direction: Direction;
  base: string;
  quote: string;
  qty: string;
  price: string;
  notional: string;
  /** "0.0" when the fee floors to zero. */
  fee: string;
  status: TradeStatus;
  reason?: string;
  updateId?: string;
  createdAt: number;
  settledAt?: number;
}

export interface Balance {
  symbol: string;
  available: string;
  locked: string;
}

export interface IncomingTransfer {
  contractId: string;
  /** The sender's party hint. */
  sender: string;
  symbol: string;
  amount: string;
}

export type SignPurpose = "allocate" | "transfer-accept" | "transfer-out";

/** A prepared ledger transaction waiting for the party key's signature. */
export interface SignAction {
  id: string;
  purpose: SignPurpose;
  description: string;
  /** The prepared-transaction hash to sign, raw bytes. */
  hash: Uint8Array;
  createdAt: number;
  expiresAt: number;
}

export interface NewRfq {
  direction: Direction;
  qty: string;
  base: string;
  quote: string;
  makers: string[];
  ttlSeconds: number;
}

export interface NewQuote {
  price: string;
  ttlSeconds?: number;
}

export interface Withdrawal {
  symbol: string;
  amount: string;
  /** The receiver's full party id (`hint::fingerprint`). */
  receiver: string;
}
