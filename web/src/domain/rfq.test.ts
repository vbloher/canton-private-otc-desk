import { describe, expect, it } from "vitest";
import {
  bestQuote,
  canAccept,
  canCancel,
  canQuote,
  incomingRowStatus,
  makerTerms,
  QUOTE_TTL_CHOICES_S,
  QUOTE_TTL_MAX_S,
  QUOTE_TTL_MIN_S,
  quoteTerms,
  RFQ_TTL_CHOICES_S,
  RFQ_TTL_MAX_S,
  RFQ_TTL_MIN_S,
  rfqRowStatus,
} from "./rfq";
import type { Quote, Rfq, RfqWithQuotes } from "./types";

const NOW = Date.parse("2026-10-21T12:00:00Z");

function rfq(over: Partial<Rfq> = {}): Rfq {
  return {
    id: "r1",
    taker: "alice-3f9a1c",
    direction: "BUY",
    qty: "0.5",
    base: "CBTC",
    quote: "USDCx",
    makers: ["makerA", "makerB"],
    feeBps: 10,
    deadline: NOW + 60_000,
    status: "open",
    createdAt: NOW - 60_000,
    ...over,
  };
}

function quote(id: string, price: string, over: Partial<Quote> = {}): Quote {
  return {
    id,
    rfqId: "r1",
    maker: `maker-${id}`,
    price,
    validUntil: NOW + 30_000,
    status: "open",
    createdAt: NOW - 10_000,
    ...over,
  };
}

describe("bestQuote", () => {
  const quotes = [quote("a", "100150.0"), quote("b", "99850.0"), quote("c", "100000.0")];

  it("picks the lowest price when the taker buys", () => {
    expect(bestQuote(quotes, "BUY")?.id).toBe("b");
  });

  it("picks the highest price when the taker sells", () => {
    expect(bestQuote(quotes, "SELL")?.id).toBe("a");
  });

  it("ignores quotes that are not open", () => {
    const mixed = [
      quote("allocating", "1", { status: "allocating" }),
      quote("expired", "2", { status: "expired" }),
      quote("lost", "3", { status: "lost" }),
      quote("open", "100000.0"),
    ];
    expect(bestQuote(mixed, "BUY")?.id).toBe("open");
    expect(bestQuote([quote("x", "1", { status: "cancelled" })], "BUY")).toBeUndefined();
  });

  it("ignores open quotes past validUntil when a clock is given", () => {
    const stale = [quote("stale", "1", { validUntil: NOW }), quote("live", "2")];
    expect(bestQuote(stale, "BUY", NOW)?.id).toBe("live");
  });

  it("compares prices exactly at the tenth digit", () => {
    const close = [quote("hi", "100000.0000000002"), quote("lo", "100000.0000000001")];
    expect(bestQuote(close, "BUY")?.id).toBe("lo");
    expect(bestQuote(close, "SELL")?.id).toBe("hi");
  });

  it("breaks a price tie with the earlier quote", () => {
    const tie = [
      quote("later", "5", { createdAt: NOW - 1_000 }),
      quote("first", "5", { createdAt: NOW - 2_000 }),
    ];
    expect(bestQuote(tie, "BUY")?.id).toBe("first");
  });
});

describe("rfqRowStatus", () => {
  const row = (r: Partial<Rfq>, quotes: Quote[] = []): RfqWithQuotes => ({ rfq: rfq(r), quotes });

  it("reads open while the deadline runs", () => {
    expect(rfqRowStatus(row({}), NOW)).toBe("open");
  });

  it("reads expired once the deadline passed, even before the server says so", () => {
    expect(rfqRowStatus(row({ deadline: NOW }), NOW)).toBe("expired");
    expect(rfqRowStatus(row({ status: "expired" }), NOW)).toBe("expired");
  });

  it("reads settling while an accepted quote settles, past the deadline too", () => {
    expect(rfqRowStatus(row({ deadline: NOW - 1 }, [quote("q", "1", { status: "accepted" })]), NOW)).toBe(
      "settling",
    );
  });

  it("reads filled and cancelled from the server", () => {
    expect(rfqRowStatus(row({ status: "filled" }, [quote("q", "1", { status: "won" })]), NOW)).toBe("filled");
    expect(rfqRowStatus(row({ status: "cancelled" }), NOW)).toBe("cancelled");
  });
});

describe("incomingRowStatus", () => {
  it("asks for a quote while the RFQ is open and unquoted", () => {
    expect(incomingRowStatus({ rfq: rfq() }, NOW)).toBe("new");
  });

  it("shows the maker's own quote status", () => {
    expect(incomingRowStatus({ rfq: rfq(), myQuote: quote("q", "1", { status: "allocating" }) }, NOW)).toBe(
      "allocating",
    );
    expect(incomingRowStatus({ rfq: rfq(), myQuote: quote("q", "1", { status: "lost" }) }, NOW)).toBe("lost");
  });

  it("reads an open quote past validUntil as expired", () => {
    expect(incomingRowStatus({ rfq: rfq(), myQuote: quote("q", "1", { validUntil: NOW }) }, NOW)).toBe("expired");
  });

  it("reads an unquoted RFQ that ended as expired or closed", () => {
    expect(incomingRowStatus({ rfq: rfq({ deadline: NOW }) }, NOW)).toBe("expired");
    expect(incomingRowStatus({ rfq: rfq({ status: "filled" }) }, NOW)).toBe("closed");
    expect(incomingRowStatus({ rfq: rfq({ status: "cancelled" }) }, NOW)).toBe("closed");
  });
});

describe("taker and maker actions", () => {
  it("accepts only an open, unexpired quote on an open RFQ without a trade", () => {
    const live = quote("q", "1");
    expect(canAccept({ rfq: rfq(), quotes: [live] }, live, NOW)).toBe(true);
    expect(canAccept({ rfq: rfq(), quotes: [live] }, { ...live, validUntil: NOW }, NOW)).toBe(false);
    expect(canAccept({ rfq: rfq({ deadline: NOW }), quotes: [live] }, live, NOW)).toBe(false);
    const accepted = quote("other", "1", { status: "accepted" });
    expect(canAccept({ rfq: rfq(), quotes: [live, accepted] }, live, NOW)).toBe(false);
  });

  it("cancels an open RFQ unless a trade on it is settling", () => {
    expect(canCancel({ rfq: rfq(), quotes: [] }, NOW)).toBe(true);
    expect(canCancel({ rfq: rfq(), quotes: [quote("q", "1", { status: "accepted" })] }, NOW)).toBe(false);
    expect(canCancel({ rfq: rfq({ status: "cancelled" }), quotes: [] }, NOW)).toBe(false);
  });

  it("quotes once per RFQ, while it is open", () => {
    expect(canQuote({ rfq: rfq() }, NOW)).toBe(true);
    expect(canQuote({ rfq: rfq(), myQuote: quote("q", "1", { status: "expired" }) }, NOW)).toBe(false);
    expect(canQuote({ rfq: rfq({ deadline: NOW }) }, NOW)).toBe(false);
  });
});

describe("quoteTerms", () => {
  it("floors the notional, then takes the fee from the floored notional", () => {
    expect(quoteTerms("0.5", "100000", 10)).toEqual({ notional: "50000.0", fee: "50.0" });
    expect(quoteTerms("0.3333333333", "3", 10)).toEqual({ notional: "0.9999999999", fee: "0.0009999999" });
  });

  it("flags dust when the quote leg floors to zero", () => {
    expect(quoteTerms("0.0000000001", "0.5", 10)).toEqual({ notional: "0.0", fee: "0.0" });
  });
});

describe("makerTerms", () => {
  it("when the taker buys, the maker delivers the base, receives the notional less the fee, and commits the base and the fee", () => {
    expect(makerTerms({ direction: "BUY", qty: "0.5", feeBps: 10 }, "100000")).toEqual({
      notional: "50000.0",
      fee: "50.0",
      deliversBase: true,
      quoteTotal: "49950.0",
      commitsBase: "0.5",
      commitsQuote: "50.0",
    });
  });

  it("when the taker sells, the maker receives the base and pays the notional plus the fee, all committed up front", () => {
    expect(makerTerms({ direction: "SELL", qty: "2", feeBps: 10 }, "3995")).toEqual({
      notional: "7990.0",
      fee: "7.99",
      deliversBase: false,
      quoteTotal: "7997.99",
      commitsBase: "0.0",
      commitsQuote: "7997.99",
    });
  });
});

describe("TTL choices", () => {
  it("offers only lifetimes the trading API accepts", () => {
    for (const s of RFQ_TTL_CHOICES_S) expect(s >= RFQ_TTL_MIN_S && s <= RFQ_TTL_MAX_S).toBe(true);
    for (const s of QUOTE_TTL_CHOICES_S) expect(s >= QUOTE_TTL_MIN_S && s <= QUOTE_TTL_MAX_S).toBe(true);
  });
});
