/**
 * Cache keys of every server read. Prefixes group them: invalidating ["rfqs"]
 * refreshes mine, incoming and history at once.
 */
export type QueryKey = readonly string[];

export const queryKeys = {
  me: ["me"],
  makers: ["makers"],
  desk: ["desk"],
  instruments: ["instruments"],
  rfqs: ["rfqs"],
  myRfqs: ["rfqs", "mine"],
  incomingRfqs: ["rfqs", "incoming"],
  rfqHistory: ["rfqs", "history"],
  trades: ["trades"],
  wallet: ["wallet"],
  balances: ["wallet", "balances"],
  incomingTransfers: ["wallet", "incoming"],
  pendingActions: ["tx", "pending"],
} as const satisfies Record<string, QueryKey>;
