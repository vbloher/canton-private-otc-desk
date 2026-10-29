import { addDecimal, cmpDecimal, formatDecimal, mulBps, mulDecimal, subDecimal } from "./money";
import type { Direction, IncomingRfq, Quote, QuoteStatus, Rfq, RfqWithQuotes } from "./types";

export const RFQ_TTL_MIN_S = 30;
export const RFQ_TTL_MAX_S = 3600;
export const RFQ_TTL_CHOICES_S = [60, 300, 900, 3600] as const;
export const RFQ_TTL_DEFAULT_S = 300;
export const RFQ_MAKERS_MAX = 10;
export const QUOTE_TTL_MIN_S = 10;
export const QUOTE_TTL_MAX_S = 600;
export const QUOTE_TTL_CHOICES_S = [30, 60, 120, 300] as const;
export const QUOTE_TTL_DEFAULT_S = 60;

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

export function rfqRowStatus({ rfq, quotes }: RfqWithQuotes, now: number): RfqRowStatus {
  if (rfq.status === "filled" || quotes.some((q) => q.status === "won")) return "filled";
  if (quotes.some((q) => q.status === "accepted")) return "settling";
  if (rfq.status === "cancelled") return "cancelled";
  if (rfq.status === "expired" || rfq.deadline <= now) return "expired";
  return "open";
}

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

export function canAccept(entry: RfqWithQuotes, quote: Quote, now: number): boolean {
  return (
    rfqLive(entry, now) &&
    quote.status === "open" &&
    quote.validUntil > now &&
    !entry.quotes.some(settlingOrDone)
  );
}

export function canCancel(entry: RfqWithQuotes, now: number): boolean {
  return rfqLive(entry, now) && !entry.quotes.some(settlingOrDone);
}

export function canQuote(entry: IncomingRfq, now: number): boolean {
  return rfqLive(entry, now) && entry.myQuote === undefined;
}

export interface QuoteTerms {
  notional: string;
  fee: string;
}

export function quoteTerms(qty: string, price: string, feeBps: number): QuoteTerms {
  const notional = mulDecimal(qty, price);
  return { notional, fee: mulBps(notional, feeBps) };
}

export interface MakerTerms extends QuoteTerms {
  deliversBase: boolean;
  quoteTotal: string;
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
