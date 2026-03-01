"use client";

import { createContext, useContext } from "react";
import type { DocumentSummary } from "@/app/lib/knowhow/types";

const DocumentsContext = createContext<DocumentSummary[] | null>(null);

export function DocumentsProvider({
  documents,
  children,
}: {
  documents: DocumentSummary[];
  children: React.ReactNode;
}) {
  return <DocumentsContext value={documents}>{children}</DocumentsContext>;
}

export function useDocuments(): DocumentSummary[] {
  const ctx = useContext(DocumentsContext);
  if (ctx === null) {
    throw new Error("useDocuments must be used within a DocumentsProvider");
  }
  return ctx;
}
