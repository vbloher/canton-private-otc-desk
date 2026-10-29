import { describe, expect, it } from "vitest";
import { invalidationsFor, lateInvalidationsFor } from "./event-invalidation";

describe("lateInvalidationsFor", () => {
  it.each([
    ["sign.completed", [["wallet"]]],
    ["wallet.updated", [["wallet"]]],
    ["rfq.created", []],
    ["trade.updated", []],
    ["desk.restarted", []],
  ])("%s refreshes %j once more a moment later", (type, keys) => {
    expect(lateInvalidationsFor(type)).toEqual(keys);
  });
});

describe("invalidationsFor", () => {
  it.each([
    ["rfq.created", [["rfqs"]]],
    ["rfq.updated", [["rfqs"]]],
    ["quote.updated", [["rfqs"], ["wallet", "balances"]]],
    ["trade.updated", [["rfqs"], ["trades"], ["wallet", "balances"]]],
    ["sign.requested", [["tx", "pending"]]],
    ["sign.completed", [["tx", "pending"], ["wallet"]]],
    ["wallet.updated", [["wallet"]]],
  ])("%s refreshes %j", (type, keys) => {
    expect(invalidationsFor(type)).toEqual(keys);
  });

  it("ignores event types it does not know", () => {
    expect(invalidationsFor("desk.restarted")).toEqual([]);
    expect(invalidationsFor("constructor")).toEqual([]);
  });
});
