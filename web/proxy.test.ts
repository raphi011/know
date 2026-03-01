import { describe, it, expect } from "vitest";
import { isValidReturnTo } from "./proxy";

describe("isValidReturnTo", () => {
  it.each([
    ["/dashboard", true],
    ["/settings/profile", true],
    ["/", true],
    ["/path?query=1", true],
    ["/path#anchor", true],
  ])("accepts valid relative path: %s", (input, expected) => {
    expect(isValidReturnTo(input)).toBe(expected);
  });

  it.each([
    ["//evil.com", false, "protocol-relative URL"],
    ["//evil.com/path", false, "protocol-relative with path"],
    ["https://evil.com", false, "absolute URL with scheme"],
    ["javascript:alert(1)", false, "javascript protocol"],
    ["data:text/html,<h1>X</h1>", false, "data URI"],
    ["", false, "empty string"],
    ["relative/path", false, "relative path without leading slash"],
    ["/\\evil.com", false, "backslash (treated as // in some browsers)"],
    ["/%2F/evil.com", false, "encoded double slash"],
    ["/%5Cevil.com", false, "encoded backslash"],
    ["/path%0d%0aLocation: http://evil.com", false, "CRLF header injection"],
    ["/path%0aSet-Cookie: x=y", false, "LF header injection"],
    ["/path%3Aevil", false, "encoded colon"],
    ["%2F%2Fevil.com", false, "fully encoded protocol-relative"],
    ["/%%invalid", false, "malformed percent-encoding"],
  ])("rejects %s (%s)", (input, expected) => {
    expect(isValidReturnTo(input)).toBe(expected);
  });
});
