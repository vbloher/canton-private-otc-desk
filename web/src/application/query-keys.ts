export type QueryKey = readonly string[];

export const queryKeys = {
  me: ["me"],
  makers: ["makers"],
  desk: ["desk"],
  instruments: ["instruments"],
  network: ["network"],
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
