"use client";

import { useState } from "react";
import { DocumentTextIcon } from "@heroicons/react/20/solid";
import { useDocuments } from "@/components/domain/documents-context";
import { cn } from "@/lib/utils";

type DocRefAutocompleteProps = {
  onSelect: (path: string) => void;
  onClose: () => void;
};

function DocRefAutocomplete({ onSelect, onClose }: DocRefAutocompleteProps) {
  const documents = useDocuments();
  const [filter, setFilter] = useState("");

  const filtered = documents.filter(
    (doc) =>
      doc.path.toLowerCase().includes(filter.toLowerCase()) ||
      doc.title.toLowerCase().includes(filter.toLowerCase()),
  );

  return (
    <div className="mb-1.5 overflow-hidden rounded-lg ring-1 ring-slate-200 dark:ring-slate-700">
      <input
        type="text"
        value={filter}
        onChange={(e) => setFilter(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Escape") onClose();
          if (e.key === "Enter" && filtered.length > 0) {
            e.preventDefault();
            onSelect(filtered[0]!.path);
          }
        }}
        placeholder="Search documents..."
        autoFocus
        className={cn(
          "w-full border-b border-slate-200 bg-white px-2.5 py-1.5 text-xs",
          "text-slate-900 placeholder:text-slate-400",
          "focus:outline-none",
          "dark:border-slate-700 dark:bg-slate-900 dark:text-white",
        )}
      />
      <div className="max-h-[120px] overflow-y-auto">
        {filtered.slice(0, 8).map((doc) => (
          <button
            key={doc.path}
            type="button"
            onClick={() => onSelect(doc.path)}
            className={cn(
              "flex w-full items-center gap-1.5 px-2.5 py-1.5 text-left text-xs",
              "text-slate-700 hover:bg-slate-50",
              "dark:text-slate-300 dark:hover:bg-slate-800",
            )}
          >
            <DocumentTextIcon className="size-3 shrink-0 text-slate-400" />
            <span className="truncate">{doc.title}</span>
            <span className="ml-auto truncate text-[10px] text-slate-400">
              {doc.path}
            </span>
          </button>
        ))}
        {filtered.length === 0 && (
          <div className="px-2.5 py-2 text-xs text-slate-400">
            No documents found
          </div>
        )}
      </div>
    </div>
  );
}

export { DocRefAutocomplete };
