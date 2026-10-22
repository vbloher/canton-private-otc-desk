import { describe, expect, it } from "vitest";
import { sanitizeDecimal } from "./amount";

describe("sanitizeDecimal", () => {
  it.each([
    ["abc1.2.3", "1.23"],
    ["0.123456789012", "0.1234567890"],
    [" 12 000 ", "12000"],
    ["", ""],
    [".5", "0.5"],
    ["1".repeat(30), "1".repeat(28)],
  ])("cleans %j to %j", (typed, kept) => {
    expect(sanitizeDecimal(typed)).toBe(kept);
  });

  it.each([
    ["100,000", "100000"],
    ["100,000.5", "100000.5"],
    ["1,000,000.25", "1000000.25"],
    ["1,5", "15"],
  ])("reads commas as thousands separators: %j is %j", (typed, kept) => {
    expect(sanitizeDecimal(typed)).toBe(kept);
  });
});
