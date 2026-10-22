import { describe, expect, it } from "vitest";
import { activityFeed, viewTrade } from "./trade";
import type { Rfq, Trade } from "./types";

const T0 = Date.parse("2026-10-21T12:00:00Z");

function trade(over: Partial<Trade> = {}): Trade {
  return {
    id: "t1",
    rfqId: "r1",
    quoteId: "q1",
    taker: "alice-3f9a1c",
    maker: "makerA",
    direction: "BUY",
    base: "CBTC",
    quote: "USDCx",
    qty: "0.5",
    price: "100000.0",
    notional: "50000.0",
    fee: "50.0",
    status: "settled",
    updateId: "1220aa",
    createdAt: T0,
    settledAt: T0 + 5_000,
    ...over,
  };
}

function closedRfq(id: string, createdAt: number): Rfq {
  return {
    id,
    taker: "alice-3f9a1c",
    direction: "SELL",
    qty: "1.0",
    base: "CETH",
    quote: "USDCx",
    makers: ["makerA"],
    feeBps: 10,
    deadline: createdAt + 300_000,
    status: "expired",
    createdAt,
  };
}

describe("viewTrade", () => {
  it("a buying taker gets the base and gives the notional, without a fee", () => {
    expect(viewTrade(trade(), "alice-3f9a1c")).toEqual({
      role: "taker",
      boughtBase: true,
      got: { symbol: "CBTC", amount: "0.5" },
      gave: { symbol: "USDCx", amount: "50000.0" },
      counterparty: "makerA",
    });
  });

  it("a selling taker gives the base and gets the notional", () => {
    const v = viewTrade(trade({ direction: "SELL" }), "alice-3f9a1c");
    expect(v.boughtBase).toBe(false);
    expect(v.gave).toEqual({ symbol: "CBTC", amount: "0.5" });
    expect(v.got).toEqual({ symbol: "USDCx", amount: "50000.0" });
    expect(v.fee).toBeUndefined();
  });

  it("the maker takes the other side and pays the fee in the quote asset", () => {
    expect(viewTrade(trade(), "makerA")).toEqual({
      role: "maker",
      boughtBase: false,
      gave: { symbol: "CBTC", amount: "0.5" },
      got: { symbol: "USDCx", amount: "50000.0" },
      fee: { symbol: "USDCx", amount: "50.0" },
      counterparty: "alice-3f9a1c",
    });
  });

  it("shows no fee when it floored to zero", () => {
    expect(viewTrade(trade({ fee: "0.0" }), "makerA").fee).toBeUndefined();
  });
});

describe("activityFeed", () => {
  it("merges trades and closed RFQs newest first", () => {
    const feed = activityFeed(
      [trade({ id: "old", createdAt: T0, settledAt: T0 + 1_000 }), trade({ id: "new", createdAt: T0 + 9_000, settledAt: undefined, status: "settling" })],
      [closedRfq("r-mid", T0 + 5_000)],
      "alice-3f9a1c",
    );
    expect(feed.map((i) => i.key)).toEqual(["trade:new", "rfq:r-mid", "trade:old"]);
  });

  it("dates a settled trade by its settlement", () => {
    const [item] = activityFeed([trade()], [], "alice-3f9a1c");
    expect(item?.at).toBe(T0 + 5_000);
  });
});
