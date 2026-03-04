import { describe, it, expect } from "vitest";
import {
  resolveDropPath,
  hasNameConflict,
  isDescendantPath,
  validateInternalDrop,
  filterMarkdownFiles,
} from "./dnd-utils";
import type { DocumentSummary } from "./types";

describe("resolveDropPath", () => {
  it("moves doc to root", () => {
    expect(resolveDropPath("folder/doc.md", "")).toBe("doc.md");
  });
  it("moves doc into folder", () => {
    expect(resolveDropPath("doc.md", "folder")).toBe("folder/doc.md");
  });
  it("moves doc between folders", () => {
    expect(resolveDropPath("a/doc.md", "b")).toBe("b/doc.md");
  });
  it("moves folder to root", () => {
    expect(resolveDropPath("parent/child", "")).toBe("child");
  });
  it("moves folder into folder", () => {
    expect(resolveDropPath("child", "parent")).toBe("parent/child");
  });
});

describe("hasNameConflict", () => {
  const docs: DocumentSummary[] = [
    { id: "1", vaultId: "v", path: "readme.md", title: "", labels: [], docType: null, createdAt: "", updatedAt: "" },
    { id: "2", vaultId: "v", path: "folder/notes.md", title: "", labels: [], docType: null, createdAt: "", updatedAt: "" },
  ];

  it("detects conflict at root", () => {
    expect(hasNameConflict(docs, "readme.md")).toBe(true);
  });
  it("detects conflict in folder", () => {
    expect(hasNameConflict(docs, "folder/notes.md")).toBe(true);
  });
  it("no conflict for new path", () => {
    expect(hasNameConflict(docs, "folder/new.md")).toBe(false);
  });
  it("case-insensitive", () => {
    expect(hasNameConflict(docs, "README.md")).toBe(true);
  });
});

describe("isDescendantPath", () => {
  it("direct child is descendant", () => {
    expect(isDescendantPath("parent", "parent/child")).toBe(true);
  });
  it("nested descendant", () => {
    expect(isDescendantPath("a", "a/b/c")).toBe(true);
  });
  it("not a descendant", () => {
    expect(isDescendantPath("a", "b/c")).toBe(false);
  });
  it("same path is descendant", () => {
    expect(isDescendantPath("a", "a")).toBe(true);
  });
  it("partial name match is not descendant", () => {
    expect(isDescendantPath("app", "application/file.md")).toBe(false);
  });
});

describe("validateInternalDrop", () => {
  it("rejects drop on self", () => {
    expect(validateInternalDrop("a", "a").valid).toBe(false);
  });
  it("rejects folder drop on own descendant", () => {
    expect(validateInternalDrop("a", "a/b").valid).toBe(false);
  });
  it("rejects drop on same parent (doc already there)", () => {
    expect(validateInternalDrop("folder/doc.md", "folder").valid).toBe(false);
  });
  it("allows doc move to different folder", () => {
    expect(validateInternalDrop("a/doc.md", "b").valid).toBe(true);
  });
  it("allows doc move to root", () => {
    expect(validateInternalDrop("folder/doc.md", "").valid).toBe(true);
  });
  it("allows folder move to different folder", () => {
    expect(validateInternalDrop("a", "b").valid).toBe(true);
  });
});

describe("filterMarkdownFiles", () => {
  const makeFile = (name: string) => new File(["content"], name);

  it("accepts .md files", () => {
    const files = [makeFile("readme.md"), makeFile("notes.md")];
    const result = filterMarkdownFiles(files);
    expect(result.valid).toHaveLength(2);
    expect(result.skipped).toBe(0);
  });

  it("rejects non-.md files", () => {
    const files = [makeFile("image.png"), makeFile("readme.md")];
    const result = filterMarkdownFiles(files);
    expect(result.valid).toHaveLength(1);
    expect(result.valid[0]!.name).toBe("readme.md");
    expect(result.skipped).toBe(1);
  });

  it("handles empty list", () => {
    const result = filterMarkdownFiles([]);
    expect(result.valid).toHaveLength(0);
    expect(result.skipped).toBe(0);
  });
});
