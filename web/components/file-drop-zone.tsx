"use client";

import { useCallback, useState, type DragEvent, type ReactNode } from "react";
import {
  filterMarkdownFiles,
  hasNameConflict,
  resolveDropPath,
} from "@/app/lib/knowhow/dnd-utils";
import { createDocument } from "@/app/lib/knowhow/mutations";
import { useToast } from "@/components/ui/toast-provider";
import type { DocumentSummary } from "@/app/lib/knowhow/types";

interface FileDropZoneProps {
  vaultId: string;
  targetFolderPath: string;
  documents: DocumentSummary[];
  onImportComplete: () => void;
  children: ReactNode;
}

export function FileDropZone({
  vaultId,
  targetFolderPath,
  documents,
  onImportComplete,
  children,
}: FileDropZoneProps) {
  const [isDragOver, setIsDragOver] = useState(false);
  const [isImporting, setIsImporting] = useState(false);
  const { toast } = useToast();

  const handleDragEnter = useCallback((e: DragEvent) => {
    e.preventDefault();
    if (e.dataTransfer.types.includes("Files")) {
      setIsDragOver(true);
    }
  }, []);

  const handleDragOver = useCallback((e: DragEvent) => {
    e.preventDefault();
    e.dataTransfer.dropEffect = "copy";
  }, []);

  const handleDragLeave = useCallback((e: DragEvent) => {
    // Only clear when leaving the container itself, not child elements
    if (!e.currentTarget.contains(e.relatedTarget as Node)) {
      setIsDragOver(false);
    }
  }, []);

  const handleDrop = useCallback(
    async (e: DragEvent) => {
      e.preventDefault();
      setIsDragOver(false);

      const files = Array.from(e.dataTransfer.files);
      const { valid, skipped } = filterMarkdownFiles(files);

      if (skipped > 0) {
        toast({
          variant: "info",
          title: `Skipped ${skipped} non-markdown file${skipped > 1 ? "s" : ""}`,
        });
      }

      if (valid.length === 0) return;

      setIsImporting(true);
      let imported = 0;

      for (const file of valid) {
        const path = resolveDropPath(file.name, targetFolderPath);

        if (hasNameConflict(documents, path)) {
          toast({
            variant: "error",
            title: `"${file.name}" already exists in ${targetFolderPath || "root"}`,
          });
          continue;
        }

        const content = await file.text();
        const result = await createDocument(vaultId, path, content);

        if (result.success) {
          imported++;
        } else {
          toast({ variant: "error", title: `Failed to import ${file.name}` });
        }
      }

      setIsImporting(false);

      if (imported > 0) {
        toast({
          variant: "success",
          title: `Imported ${imported} file${imported > 1 ? "s" : ""}`,
        });
        onImportComplete();
      }
    },
    [vaultId, targetFolderPath, documents, toast, onImportComplete],
  );

  return (
    <div
      className="relative"
      onDragEnter={handleDragEnter}
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
    >
      {children}
      {isDragOver && (
        <div className="pointer-events-none absolute inset-0 z-40 flex items-center justify-center rounded-lg border-2 border-dashed border-blue-400 bg-blue-50/80 dark:border-blue-600 dark:bg-blue-950/80">
          <p className="text-sm font-medium text-blue-600 dark:text-blue-400">
            Drop .md files here
          </p>
        </div>
      )}
      {isImporting && (
        <div className="pointer-events-none absolute inset-0 z-40 flex items-center justify-center rounded-lg bg-white/60 dark:bg-zinc-900/60">
          <p className="text-sm text-zinc-500">Importing...</p>
        </div>
      )}
    </div>
  );
}
