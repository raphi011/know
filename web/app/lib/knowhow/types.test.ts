import { describe, expect, it } from "vitest";
import { graphqlUrl, stripGraphqlPath } from "./types";

describe("graphqlUrl", () => {
  it("appends /query to base URL", () => {
    expect(graphqlUrl("http://localhost:8484")).toBe(
      "http://localhost:8484/query",
    );
  });
});

describe("stripGraphqlPath", () => {
  it("strips /query suffix", () => {
    expect(stripGraphqlPath("http://localhost:8484/query")).toBe(
      "http://localhost:8484",
    );
  });

  it("strips trailing slashes", () => {
    expect(stripGraphqlPath("http://localhost:8484/")).toBe(
      "http://localhost:8484",
    );
  });

  it("strips trailing slashes then /query", () => {
    expect(stripGraphqlPath("http://localhost:8484/query/")).toBe(
      "http://localhost:8484",
    );
  });

  it("returns URL unchanged when no /query suffix", () => {
    expect(stripGraphqlPath("http://localhost:8484")).toBe(
      "http://localhost:8484",
    );
  });
});
