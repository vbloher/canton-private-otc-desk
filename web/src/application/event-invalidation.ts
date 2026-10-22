/**
 * Server events carry ids, never state: each one names the reads it made stale
 * and the UI re-fetches them. (A reconnect re-fetches everything, since events
 * are at-most-once.)
 */
import type { DeskEventType } from "./ports";
import { queryKeys, type QueryKey } from "./query-keys";

const STALE: Record<DeskEventType, readonly QueryKey[]> = {
  "rfq.created": [queryKeys.rfqs],
  "rfq.updated": [queryKeys.rfqs],
  // A quote is the maker's committed allocation: opening, losing or expiring it moves locked funds.
  "quote.updated": [queryKeys.rfqs, queryKeys.balances],
  "trade.updated": [queryKeys.rfqs, queryKeys.trades, queryKeys.balances],
  "sign.requested": [queryKeys.pendingActions],
  // An executed action changed holdings or the incoming transfers.
  "sign.completed": [queryKeys.pendingActions, queryKeys.wallet],
  "wallet.updated": [queryKeys.wallet],
};

/**
 * The ledger executes a signed action asynchronously, so the refetch an
 * execution event triggers can still read the old holdings: these reads are
 * refreshed once more, LATE_REFRESH_MS later.
 */
const STALE_LATER: Partial<Record<DeskEventType, readonly QueryKey[]>> = {
  "sign.completed": [queryKeys.wallet],
  "wallet.updated": [queryKeys.wallet],
};

export const LATE_REFRESH_MS = 2_000;

export function isDeskEventType(type: string): type is DeskEventType {
  return Object.hasOwn(STALE, type);
}

export function invalidationsFor(type: string): QueryKey[] {
  return isDeskEventType(type) ? [...STALE[type]] : [];
}

export function lateInvalidationsFor(type: string): QueryKey[] {
  return isDeskEventType(type) ? [...(STALE_LATER[type] ?? [])] : [];
}

export type { QueryKey };
