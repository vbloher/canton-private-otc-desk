import type {
  Balance,
  DeskTerms,
  IncomingRfq,
  IncomingTransfer,
  Instrument,
  Network,
  Maker,
  Me,
  NewQuote,
  NewRfq,
  Quote,
  QuoteStatus,
  Rfq,
  RfqWithQuotes,
  SignAction,
  Trade,
  TradeStatus,
  Withdrawal,
} from "@/domain/types";

export interface Registration {
  registrationId: string;
  partyId: string;
  multiHash: Uint8Array;
}

export interface LoginChallenge {
  challenge: string;
  encryptedBlob: string;
  publicKey: Uint8Array;
}

export interface AuthApi {
  registerStart(input: { displayName: string; publicKey: Uint8Array }): Promise<Registration>;
  registerParty(input: { registrationId: string; signature: Uint8Array }): Promise<string>;
  registerComplete(input: {
    registrationId: string;
    signature: Uint8Array;
    credentialId: string;
    encryptedBlob: string;
  }): Promise<Me>;
  createChallenge(credentialId: string): Promise<LoginChallenge>;
  login(input: { challenge: string; signature: Uint8Array; credentialId: string }): Promise<Me>;
  logout(): Promise<void>;
  me(): Promise<Me | null>;
  setMakerMode(enabled: boolean): Promise<Me>;
  makers(): Promise<Maker[]>;
}

export interface DeskApi {
  desk(): Promise<DeskTerms>;
  createRfq(input: NewRfq): Promise<Rfq>;
  myRfqs(): Promise<RfqWithQuotes[]>;
  incomingRfqs(): Promise<IncomingRfq[]>;
  rfqHistory(): Promise<Rfq[]>;
  cancelRfq(rfqId: string): Promise<void>;
  submitQuote(rfqId: string, input: NewQuote): Promise<Quote>;
  acceptQuote(quoteId: string): Promise<Trade>;
  trades(): Promise<Trade[]>;
}

export interface WalletApi {
  instruments(): Promise<Instrument[]>;
  network(): Promise<Network>;
  balances(): Promise<Balance[]>;
  incoming(): Promise<IncomingTransfer[]>;
  acceptIncoming(contractId: string): Promise<SignAction[]>;
  withdraw(input: Withdrawal): Promise<SignAction[]>;
  faucet(): Promise<SignAction[]>;
  pendingActions(): Promise<SignAction[]>;
  executeAction(actionId: string, signature: Uint8Array): Promise<void>;
}

export interface UnlockedPasskey {
  credentialId: string;
  wrap(seed: Uint8Array): Promise<string>;
  unwrap(blob: string): Promise<Uint8Array>;
}

export type PasskeySupport = "supported" | "unknown" | "no-prf" | "none";

export interface KeyVault {
  support(): Promise<PasskeySupport>;
  createPasskey(displayName: string): Promise<string>;
  unlock(credentialId?: string): Promise<UnlockedPasskey>;
}

export interface Signer {
  sign(message: Uint8Array): Promise<Uint8Array>;
}

export interface PartyKeys {
  generateSeed(): Uint8Array;
  publicKey(seed: Uint8Array): Promise<Uint8Array>;
  signer(seed: Uint8Array): Signer;
}

export interface CredentialStore {
  get(userId: string): string | undefined;
  set(userId: string, credentialId: string): void;
}

export type DeskEvent =
  | { type: "rfq.created"; data: { rfqId: string } }
  | { type: "rfq.updated"; data: { rfqId: string } }
  | { type: "quote.updated"; data: { quoteId: string; rfqId: string; status: QuoteStatus } }
  | { type: "trade.updated"; data: { tradeId: string; rfqId: string; status: TradeStatus } }
  | { type: "sign.requested"; data: { actionId: string } }
  | { type: "sign.completed"; data: { actionId: string } }
  | { type: "wallet.updated"; data: Record<string, never> };

export type DeskEventType = DeskEvent["type"];

export type SocketStatus = "connecting" | "live" | "offline";

export interface EventHandlers {
  onEvent(event: DeskEvent): void;
  onStatus(status: SocketStatus): void;
  onOpen?(): void;
}

export interface EventsSource {
  connect(handlers: EventHandlers): () => void;
}

export interface Services {
  auth: AuthApi;
  desk: DeskApi;
  wallet: WalletApi;
  vault: KeyVault;
  keys: PartyKeys;
  credentials: CredentialStore;
  events: EventsSource;
}
