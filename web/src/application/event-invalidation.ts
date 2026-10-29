import type { DeskEventType } from "./ports";
import { queryKeys, type QueryKey } from "./query-keys";

const STALE: Record<DeskEventType, readonly QueryKey[]> = {
  "rfq.created": [queryKeys.rfqs],
  "rfq.updated": [queryKeys.rfqs],
  "quote.updated": [queryKeys.rfqs, queryKeys.balances],
  "trade.updated": [queryKeys.rfqs, queryKeys.trades, queryKeys.balances],
  "sign.requested": [queryKeys.pendingActions],
  "sign.completed": [queryKeys.pendingActions, queryKeys.wallet],
  "wallet.updated": [queryKeys.wallet],
};

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
