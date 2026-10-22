/**
 * What the application needs from the outside world. Infrastructure implements
 * these; the composition root (main.tsx) wires them; nothing here knows about
 * HTTP, WebAuthn or WebSockets.
 */
import type {
  Balance,
  DeskTerms,
  IncomingRfq,
  IncomingTransfer,
  Instrument,
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

/** register/start: the party topology is parked; its multi-hash must be signed. */
export interface Registration {
  registrationId: string;
  partyId: string;
  multiHash: Uint8Array;
}

/** auth/challenge: a one-time string to sign plus the wrapped key of that passkey's wallet. */
export interface LoginChallenge {
  challenge: string;
  encryptedBlob: string;
  publicKey: Uint8Array;
}

export interface AuthApi {
  registerStart(input: { displayName: string; publicKey: Uint8Array }): Promise<Registration>;
  registerComplete(input: {
    registrationId: string;
    signature: Uint8Array;
    credentialId: string;
    encryptedBlob: string;
  }): Promise<Me>;
  createChallenge(credentialId: string): Promise<LoginChallenge>;
  login(input: { challenge: string; signature: Uint8Array; credentialId: string }): Promise<Me>;
  logout(): Promise<void>;
  /** The signed-in user, or null when there is no session. */
  me(): Promise<Me | null>;
  setMakerMode(enabled: boolean): Promise<Me>;
  /** Makers the caller can ask for quotes (the caller excluded). */
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

/** Every call that changes the ledger answers with the actions the owner must sign. */
export interface WalletApi {
  instruments(): Promise<Instrument[]>;
  balances(): Promise<Balance[]>;
  incoming(): Promise<IncomingTransfer[]>;
  acceptIncoming(contractId: string): Promise<SignAction[]>;
  withdraw(input: Withdrawal): Promise<SignAction[]>;
  faucet(): Promise<SignAction[]>;
  pendingActions(): Promise<SignAction[]>;
  executeAction(actionId: string, signature: Uint8Array): Promise<void>;
}

/** A passkey opened with its PRF output: the key-encryption key for the party seed. */
export interface UnlockedPasskey {
  credentialId: string;
  wrap(seed: Uint8Array): Promise<string>;
  unwrap(blob: string): Promise<Uint8Array>;
}

/** "none" = no WebAuthn here; "no-prf" = the browser says it cannot do PRF. */
export type PasskeySupport = "supported" | "unknown" | "no-prf" | "none";

export interface KeyVault {
  support(): Promise<PasskeySupport>;
  /** Create a discoverable, PRF-enabled passkey; returns its credential id. */
  createPasskey(displayName: string): Promise<string>;
  /** PRF-unlock a passkey; without an id the platform offers every discoverable one. */
  unlock(credentialId?: string): Promise<UnlockedPasskey>;
}

export interface Signer {
  sign(message: Uint8Array): Promise<Uint8Array>;
}

/** Ed25519 party keys: a 32-byte seed that never leaves the browser unwrapped. */
export interface PartyKeys {
  generateSeed(): Uint8Array;
  publicKey(seed: Uint8Array): Promise<Uint8Array>;
  signer(seed: Uint8Array): Signer;
}

/** Which passkey belongs to which user, so Unlock offers the right one. */
export interface CredentialStore {
  get(userId: string): string | undefined;
  set(userId: string, credentialId: string): void;
}

/** Server pushes: what changed, never the new state (the UI re-fetches). */
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
  /** The stream (re)opened: anything may have been missed while it was down. */
  onOpen?(): void;
}

export interface EventsSource {
  /** Subscribe until the returned function is called; reconnects on its own. */
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
