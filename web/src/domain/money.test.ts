import { describe, expect, it } from "vitest";
import {
  addDecimal,
  cmpDecimal,
  formatDecimal,
  isDecimal,
  isPositiveDecimal,
  mulBps,
  mulDecimal,
  parseDecimal,
  subDecimal,
} from "./money";

describe("parseDecimal", () => {
  it.each([
    ["1", 10_000_000_000n],
    ["0.1", 1_000_000_000n],
    ["100000.50", 1_000_005_000_000_000n],
    ["0.0000000001", 1n],
    ["0.0", 0n],
  ])("parses %s exactly at scale 10", (text, scaled) => {
    expect(parseDecimal(text)).toBe(scaled);
  });

  it.each([
    "",
    "1e5",
    "-1",
    "1,5",
    " 1",
    "1 ",
    ".5",
    "1.",
    "0x10",
    "0.00000000001",
    "1" + "0".repeat(28),
  ])("rejects %j", (text) => {
    expect(() => parseDecimal(text)).toThrow(/decimal/);
  });
});

describe("isDecimal / isPositiveDecimal", () => {
  it("accepts the wire format only", () => {
    expect(isDecimal("5")).toBe(true);
    expect(isDecimal("0.0")).toBe(true);
    expect(isDecimal("1e5")).toBe(false);
    expect(isDecimal("1.5 ")).toBe(false);
  });

  it("requires a value above zero for positivity", () => {
    expect(isPositiveDecimal("0")).toBe(false);
    expect(isPositiveDecimal("0.0000000000")).toBe(false);
    expect(isPositiveDecimal("0.0000000001")).toBe(true);
    expect(isPositiveDecimal("1e5")).toBe(false);
  });
});

describe("formatDecimal", () => {
  it.each([
    [10_000_000_000n, "1.0"],
    [1n, "0.0000000001"],
    [1_000_005_000_000_000n, "100000.5"],
    [0n, "0.0"],
    [-15_000_000_000n, "-1.5"],
  ])("prints %s canonically as %s", (scaled, text) => {
    expect(formatDecimal(scaled)).toBe(text);
  });
});

describe("mulDecimal", () => {
  it("floors the product to ten fraction digits", () => {
    expect(mulDecimal("0.3333333333", "3")).toBe("0.9999999999");
    expect(mulDecimal("0.0000000001", "0.5")).toBe("0.0");
    expect(mulDecimal("0.12345", "0.00001")).toBe("0.0000012345");
  });

  it("multiplies exactly beyond float precision", () => {
    expect(mulDecimal("1.5", "100000")).toBe("150000.0");
    expect(mulDecimal("123456789012345678.1234567891", "1")).toBe("123456789012345678.1234567891");
  });
});

describe("mulBps", () => {
  it("takes basis points of an amount and floors", () => {
    expect(mulBps("50000", 10)).toBe("50.0");
    expect(mulBps("100000", 10)).toBe("100.0");
    expect(mulBps("0.0000001", 10)).toBe("0.0000000001");
    expect(mulBps("0.0000000001", 10)).toBe("0.0");
    expect(mulBps("1234.5", 0)).toBe("0.0");
  });
});

describe("cmpDecimal", () => {
  it("compares numerically, not as strings or floats", () => {
    expect(cmpDecimal("2", "2.0000000000")).toBe(0);
    expect(cmpDecimal("2", "3")).toBe(-1);
    expect(cmpDecimal("10", "9.9999999999")).toBe(1);
    expect(cmpDecimal("0.0000000002", "0.0000000001")).toBe(1);
    expect(cmpDecimal("9007199254740993", "9007199254740992")).toBe(1);
  });
});

describe("addDecimal / subDecimal", () => {
  it("adds and subtracts exactly", () => {
    expect(addDecimal("50000.0", "50.0")).toBe("50050.0");
    expect(addDecimal("0.1", "0.2")).toBe("0.3");
    expect(subDecimal("50000", "50")).toBe("49950.0");
    expect(subDecimal("1", "2")).toBe("-1.0");
  });
});
