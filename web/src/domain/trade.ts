/**
 * A trade from the viewer's side, and the activity feed built from trades and
 * RFQs that closed without one. Directions are the taker's: the maker always
 * takes the other side and pays the desk fee in the quote asset.
 */
import { isPositiveDecimal } from "./money";
import type { Rfq, Trade } from "./types";

export interface Leg {
  symbol: string;
  amount: string;
}

export interface TradeView {
  role: "taker" | "maker";
  /** The viewer received the base. */
  boughtBase: boolean;
  gave: Leg;
  got: Leg;
  /** The maker's fee; absent for the taker and when it floored to zero. */
  fee?: Leg;
  counterparty: string;
}

export function viewTrade(trade: Trade, myHint: string): TradeView {
  const role = trade.taker === myHint ? "taker" : "maker";
  const boughtBase = role === "taker" ? trade.direction === "BUY" : trade.direction === "SELL";
  const base: Leg = { symbol: trade.base, amount: trade.qty };
  const quote: Leg = { symbol: trade.quote, amount: trade.notional };
  const view: TradeView = {
    role,
    boughtBase,
    gave: boughtBase ? quote : base,
    got: boughtBase ? base : quote,
    counterparty: role === "taker" ? trade.maker : trade.taker,
  };
  if (role === "maker" && isPositiveDecimal(trade.fee)) view.fee = { symbol: trade.quote, amount: trade.fee };
  return view;
}

export type ActivityItem =
  | { kind: "trade"; key: string; at: number; trade: Trade; view: TradeView }
  | { kind: "rfq"; key: string; at: number; rfq: Rfq };

/** Trades (dated by settlement once settled) and unfilled RFQs, newest first. */
export function activityFeed(trades: readonly Trade[], closed: readonly Rfq[], myHint: string): ActivityItem[] {
  const items: ActivityItem[] = [
    ...trades.map(
      (t): ActivityItem => ({
        kind: "trade",
        key: `trade:${t.id}`,
        at: t.settledAt ?? t.createdAt,
        trade: t,
        view: viewTrade(t, myHint),
      }),
    ),
    ...closed.map((r): ActivityItem => ({ kind: "rfq", key: `rfq:${r.id}`, at: r.createdAt, rfq: r })),
  ];
  return items.sort((a, b) => b.at - a.at || (a.key < b.key ? -1 : a.key > b.key ? 1 : 0));
}
