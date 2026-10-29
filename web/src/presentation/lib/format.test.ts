import { describe, expect, it } from "vitest";
import { formatAmount, formatCountdown, formatTtl, shortId } from "./format";

describe("formatAmount", () => {
  it.each([
    ["97300.0", 2, "97,300"],
    ["1.23456789", 8, "1.23456789"],
    ["50000.1234", 2, "50,000.12"],
    ["50000.1299", 2, "50,000.12"],
    ["1234567.5", 0, "1,234,567"],
    ["0.0", 2, "0"],
    ["-1.5", 2, "-1.5"],
    ["123456789012345678.1234567891", 10, "123,456,789,012,345,678.1234567891"],
  ])("formats %s with %i digits as %s", (value, decimals, text) => {
    expect(formatAmount(value, decimals)).toBe(text);
  });

  it("never shows a non-zero amount as zero", () => {
    expect(formatAmount("0.000000005", 8)).toBe("0.000000005");
    expect(formatAmount("0.001", 2)).toBe("0.001");
  });

  it("shows up to ten digits by default", () => {
    expect(formatAmount("100150.0000000001")).toBe("100,150.0000000001");
  });
});

describe("formatCountdown", () => {
  it.each([
    [0, "0s"],
    [45_000, "45s"],
    [44_001, "45s"],
    [125_000, "2m 05s"],
    [3_725_000, "1h 02m"],
  ])("reads %i ms as %s", (ms, text) => {
    expect(formatCountdown(ms)).toBe(text);
  });
});

describe("shortId", () => {
  it("keeps short ids and elides the middle of long ones", () => {
    expect(shortId("makerA")).toBe("makerA");
    expect(shortId("1220abcdef0123456789abcdef")).toBe("1220abcd…abcdef");
  });
});

describe("formatTtl", () => {
  it.each([
    [30, "30 s"],
    [60, "1 min"],
    [120, "2 min"],
    [900, "15 min"],
    [3600, "1 h"],
    [5400, "90 min"],
  ])("labels %i s as %s", (seconds, label) => {
    expect(formatTtl(seconds)).toBe(label);
  });
});
