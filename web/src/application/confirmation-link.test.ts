import { describe, expect, it } from "vitest";
import { tokenFromHash } from "./confirmation-link";

describe("tokenFromHash", () => {
  it("reads the token from the link's fragment", () => {
    expect(tokenFromHash("#token=abc-DEF_123")).toBe("abc-DEF_123");
  });

  it.each(["", "#", "#other=1", "#token="])("finds no token in %j", (hash) => {
    expect(tokenFromHash(hash)).toBe("");
  });
});
