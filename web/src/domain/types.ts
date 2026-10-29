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
  feeBps: number;
}

export interface Instrument {
  symbol: string;
  name: string;
  decimals: number;
}

export type NetworkName = "devnet" | "testnet" | "mainnet";

export interface Drip {
  symbol: string;
  amount: string;
}

export interface Network {
  name: NetworkName;
  faucet: Drip[];
}

export type RfqStatus = "open" | "filled" | "expired" | "cancelled";

export interface Rfq {
  id: string;
  taker: string;
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
  price: string;
  validUntil: number;
  status: QuoteStatus;
  createdAt: number;
}

export interface RfqWithQuotes {
  rfq: Rfq;
  quotes: Quote[];
}

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
  sender: string;
  symbol: string;
  amount: string;
}

export type SignPurpose = "allocate" | "transfer-accept" | "transfer-out";

export interface SignAction {
  id: string;
  purpose: SignPurpose;
  description: string;
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
  receiver: string;
}
