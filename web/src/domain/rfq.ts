/**
 * RFQ rules: which quote is best for the taker, what a row reads, and which
 * actions are still possible. The server enforces all of it; these mirror it so
 * the UI never offers a dead button.
 */
import { addDecimal, cmpDecimal, formatDecimal, mulBps, mulDecimal, subDecimal } from "./money";
import type { Direction, IncomingRfq, Quote, QuoteStatus, Rfq, RfqWithQuotes } from "./types";

/** RFQ lifetime bounds (seconds), as the trading API enforces them. */
export const RFQ_TTL_MIN_S = 30;
export const RFQ_TTL_MAX_S = 3600;
/** The quote windows a taker picks from. */
export const RFQ_TTL_CHOICES_S = [60, 300, 900, 3600] as const;
export const RFQ_TTL_DEFAULT_S = 300;
export const RFQ_MAKERS_MAX = 10;
/** Quote lifetime bounds (seconds); the server clamps validUntil to the RFQ deadline. */
export const QUOTE_TTL_MIN_S = 10;
export const QUOTE_TTL_MAX_S = 600;
/** The lifetimes a maker picks from. */
export const QUOTE_TTL_CHOICES_S = [30, 60, 120, 300] as const;
export const QUOTE_TTL_DEFAULT_S = 60;

/**
 * The quote the taker should take: the lowest price when buying the base, the
 * highest when selling it. Only open quotes count (and, given a clock, only
 * those still valid); an equal price goes to the earlier quote.
 */
export function bestQuote(quotes: readonly Quote[], direction: Direction, now?: number): Quote | undefined {
  let best: Quote | undefined;
  for (const q of quotes) {
    if (q.status !== "open") continue;
    if (now !== undefined && q.validUntil <= now) continue;
    if (best === undefined) {
      best = q;
      continue;
    }
    const cmp = cmpDecimal(q.price, best.price);
    const better = direction === "BUY" ? cmp < 0 : cmp > 0;
    if (better || (cmp === 0 && q.createdAt < best.createdAt)) best = q;
  }
  return best;
}

const settlingOrDone = (q: Quote): boolean => q.status === "accepted" || q.status === "won";

export type RfqRowStatus = "open" | "settling" | "filled" | "expired" | "cancelled";

/** What a taker's RFQ reads: filled > settling > cancelled > expired > open. */
export function rfqRowStatus({ rfq, quotes }: RfqWithQuotes, now: number): RfqRowStatus {
  if (rfq.status === "filled" || quotes.some((q) => q.status === "won")) return "filled";
  if (quotes.some((q) => q.status === "accepted")) return "settling";
  if (rfq.status === "cancelled") return "cancelled";
  if (rfq.status === "expired" || rfq.deadline <= now) return "expired";
  return "open";
}

/** "new" = open and waiting for this maker's quote; "closed" = ended without one. */
export type IncomingRowStatus = QuoteStatus | "new" | "closed";

export function incomingRowStatus({ rfq, myQuote }: IncomingRfq, now: number): IncomingRowStatus {
  if (myQuote !== undefined) {
    return myQuote.status === "open" && myQuote.validUntil <= now ? "expired" : myQuote.status;
  }
  if (rfq.status === "expired" || (rfq.status === "open" && rfq.deadline <= now)) return "expired";
  if (rfq.status !== "open") return "closed";
  return "new";
}

const rfqLive = ({ rfq }: { rfq: RfqWithQuotes["rfq"] }, now: number): boolean =>
  rfq.status === "open" && rfq.deadline > now;

/** The taker may accept an open, unexpired quote while nothing else on the RFQ settles. */
export function canAccept(entry: RfqWithQuotes, quote: Quote, now: number): boolean {
  return (
    rfqLive(entry, now) &&
    quote.status === "open" &&
    quote.validUntil > now &&
    !entry.quotes.some(settlingOrDone)
  );
}

/** An open RFQ can be cancelled, except while a trade on it is settling. */
export function canCancel(entry: RfqWithQuotes, now: number): boolean {
  return rfqLive(entry, now) && !entry.quotes.some(settlingOrDone);
}

/** A maker quotes an RFQ once, while it is open. */
export function canQuote(entry: IncomingRfq, now: number): boolean {
  return rfqLive(entry, now) && entry.myQuote === undefined;
}

export interface QuoteTerms {
  /** The quote leg: qty × price, floored. Zero means the trade is dust. */
  notional: string;
  /** The maker's desk fee in the quote asset: notional × feeBps, floored. */
  fee: string;
}

export function quoteTerms(qty: string, price: string, feeBps: number): QuoteTerms {
  const notional = mulDecimal(qty, price);
  return { notional, fee: mulBps(notional, feeBps) };
}

/** A quote from the maker's side: the maker takes the other side of the taker and pays the fee. */
export interface MakerTerms extends QuoteTerms {
  /** The taker buys the base, so the maker delivers it (otherwise the maker receives it). */
  deliversBase: boolean;
  /** The quote asset the maker ends with if it wins: receives notional − fee, or pays notional + fee. */
  quoteTotal: string;
  /**
   * What sending the quote commits on the ledger (the quote is the maker's allocation, which
   * funds the maker's sending sides): the base it delivers, and in the quote asset the fee —
   * or the notional plus the fee when the maker is the one paying.
   */
  commitsBase: string;
  commitsQuote: string;
}

export function makerTerms(rfq: Pick<Rfq, "direction" | "qty" | "feeBps">, price: string): MakerTerms {
  const { notional, fee } = quoteTerms(rfq.qty, price, rfq.feeBps);
  if (rfq.direction === "BUY") {
    return {
      notional,
      fee,
      deliversBase: true,
      quoteTotal: subDecimal(notional, fee),
      commitsBase: rfq.qty,
      commitsQuote: fee,
    };
  }
  const pays = addDecimal(notional, fee);
  return { notional, fee, deliversBase: false, quoteTotal: pays, commitsBase: formatDecimal(0n), commitsQuote: pays };
}
