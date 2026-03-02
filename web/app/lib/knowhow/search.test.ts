import { describe, expect, it } from "vitest";
import { headingPathToHash, formatHeadingPath } from "./search";

describe("headingPathToHash", () => {
  it("returns empty string for null", () => {
    expect(headingPathToHash(null)).toBe("");
  });

  it("returns empty string for empty string", () => {
    expect(headingPathToHash("")).toBe("");
  });

  it("returns slug for single heading", () => {
    expect(headingPathToHash("## Setup")).toBe("#setup");
  });

  it("returns slug of deepest heading in nested path", () => {
    expect(headingPathToHash("## Setup > ### Install")).toBe("#install");
  });

  it("handles deeply nested headings", () => {
    expect(headingPathToHash("## Setup > ### Install > #### Config")).toBe(
      "#config",
    );
  });

  it("handles heading with spaces", () => {
    expect(headingPathToHash("## Getting Started")).toBe("#getting-started");
  });

  it("returns empty string for heading with only hash marks", () => {
    expect(headingPathToHash("##")).toBe("");
  });

  it("handles heading with special characters", () => {
    expect(headingPathToHash("## What's New?")).toBe("#whats-new");
  });
});

describe("formatHeadingPath", () => {
  it("strips heading prefix from single heading", () => {
    expect(formatHeadingPath("## Setup")).toBe("Setup");
  });

  it("formats nested headings with › separator", () => {
    expect(formatHeadingPath("## Setup > ### Install")).toBe("Setup › Install");
  });

  it("handles deeply nested headings", () => {
    expect(formatHeadingPath("## Setup > ### Install > #### Config")).toBe(
      "Setup › Install › Config",
    );
  });

  it("handles single h1", () => {
    expect(formatHeadingPath("# Title")).toBe("Title");
  });
});
