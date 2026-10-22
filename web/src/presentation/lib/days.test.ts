import { describe, expect, it } from "vitest";
import { groupByDay } from "./days";

const at = (y: number, m: number, d: number, h = 12): number => new Date(y, m - 1, d, h).getTime();

describe("groupByDay", () => {
  it("splits a newest-first feed into local calendar days, labelling today and yesterday", () => {
    const now = at(2026, 10, 21, 15);
    const items = [
      { id: "a", at: at(2026, 10, 21, 14) },
      { id: "b", at: at(2026, 10, 21, 9) },
      { id: "c", at: at(2026, 10, 20, 23) },
      { id: "d", at: at(2026, 10, 18) },
    ];

    const groups = groupByDay(items, now);

    expect(groups.map((g) => [g.label, g.items.map((i) => i.id)])).toEqual([
      ["Today", ["a", "b"]],
      ["Yesterday", ["c"]],
      [new Date(at(2026, 10, 18)).toLocaleDateString(undefined, { weekday: "short", month: "short", day: "numeric" }), ["d"]],
    ]);
  });

  it("gives an empty feed no groups", () => {
    expect(groupByDay([], Date.now())).toEqual([]);
  });
});
