"use client";

import { Fragment, useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import {
  Combobox,
  ComboboxInput,
  ComboboxOption,
  ComboboxOptions,
  Dialog as HeadlessDialog,
  DialogPanel,
  Transition,
  TransitionChild,
} from "@headlessui/react";
import {
  MagnifyingGlassIcon,
  DocumentTextIcon,
} from "@heroicons/react/24/outline";
import { useTranslations } from "next-intl";
import { cn } from "@/lib/utils";
import { Skeleton } from "@/components/ui/skeleton";
import {
  searchDocuments,
  headingPathToHash,
  formatHeadingPath,
} from "@/app/lib/knowhow/search";
import type { SearchResult } from "@/app/lib/knowhow/types";

type SearchCommandPaletteProps = {
  vaultId: string;
  open: boolean;
  onClose: () => void;
};

type SelectionValue = {
  path: string;
  headingPath: string | null;
};

function ResultsContent({
  loading,
  error,
  results,
  t,
}: {
  loading: boolean;
  error: string | null;
  results: SearchResult[];
  t: (key: string) => string;
}) {
  if (loading) {
    return (
      <div className="space-y-2 p-3">
        <Skeleton className="h-12 w-full" />
        <Skeleton className="h-12 w-full" />
        <Skeleton className="h-12 w-3/4" />
      </div>
    );
  }

  if (error) {
    return (
      <p className="px-4 py-6 text-center text-sm text-red-600 dark:text-red-400">
        {error}
      </p>
    );
  }

  if (results.length === 0) {
    return (
      <p className="px-4 py-6 text-center text-sm text-slate-500">
        {t("noResults")}
      </p>
    );
  }

  return (
    <ComboboxOptions static className="max-h-72 overflow-y-auto p-2">
      {results.map((result) => {
        const chunks = result.matchedChunks;

        // No chunks or single chunk without heading — show as single option
        if (chunks.length <= 1) {
          return (
            <ComboboxOption
              key={result.documentId}
              value={{ path: result.path, headingPath: chunks[0]?.headingPath ?? null } satisfies SelectionValue}
              className={({ focus }) =>
                cn(
                  "flex cursor-default items-start gap-3 rounded-xl px-3 py-2.5",
                  "transition-colors duration-100",
                  focus ? "bg-primary-50 dark:bg-primary-950" : "",
                )
              }
            >
              <DocumentTextIcon className="mt-0.5 size-5 shrink-0 text-slate-400" />
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium text-slate-900 dark:text-white">
                  {result.title}
                </p>
                <p className="truncate text-xs text-slate-500 dark:text-slate-400">
                  {result.path}
                  {chunks[0]?.headingPath && (
                    <span className="ml-1.5 text-slate-400 dark:text-slate-500">
                      &rsaquo; {formatHeadingPath(chunks[0].headingPath)}
                    </span>
                  )}
                </p>
                {chunks[0] && (
                  <p className="mt-0.5 line-clamp-2 text-xs text-slate-400 dark:text-slate-500">
                    {chunks[0].snippet}
                  </p>
                )}
              </div>
            </ComboboxOption>
          );
        }

        // Multiple chunks — show each as a selectable sub-option
        return (
          <div key={result.documentId}>
            {/* Document header (non-selectable) */}
            <div className="flex items-start gap-3 px-3 pt-2.5 pb-1">
              <DocumentTextIcon className="mt-0.5 size-5 shrink-0 text-slate-400" />
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium text-slate-900 dark:text-white">
                  {result.title}
                </p>
                <p className="truncate text-xs text-slate-500 dark:text-slate-400">
                  {result.path}
                </p>
              </div>
            </div>
            {/* Chunk sub-options */}
            {chunks.map((chunk, i) => (
              <ComboboxOption
                key={`${result.documentId}-${i}`}
                value={{ path: result.path, headingPath: chunk.headingPath } satisfies SelectionValue}
                className={({ focus }) =>
                  cn(
                    "flex cursor-default items-start gap-2 rounded-lg py-1.5 pr-3 pl-11",
                    "transition-colors duration-100",
                    focus ? "bg-primary-50 dark:bg-primary-950" : "",
                  )
                }
              >
                <div className="min-w-0 flex-1">
                  {chunk.headingPath && (
                    <p className="truncate text-xs font-medium text-slate-600 dark:text-slate-300">
                      {formatHeadingPath(chunk.headingPath)}
                    </p>
                  )}
                  <p className="line-clamp-1 text-xs text-slate-400 dark:text-slate-500">
                    {chunk.snippet}
                  </p>
                </div>
              </ComboboxOption>
            ))}
          </div>
        );
      })}
    </ComboboxOptions>
  );
}

function SearchCommandPalette({
  vaultId,
  open,
  onClose,
}: SearchCommandPaletteProps) {
  const router = useRouter();
  const t = useTranslations("search");

  const [query, setQuery] = useState("");
  const [results, setResults] = useState<SearchResult[]>([]);
  const [searchedQuery, setSearchedQuery] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const abortRef = useRef<AbortController | null>(null);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const trimmedQuery = query.trim();
  const loading = trimmedQuery !== "" && trimmedQuery !== searchedQuery;

  // Debounced search
  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current);
    abortRef.current?.abort();

    const trimmed = query.trim();

    if (!trimmed) {
      debounceRef.current = setTimeout(() => {
        setResults([]);
        setSearchedQuery(null);
        setError(null);
      }, 0);
      return () => {
        if (debounceRef.current) clearTimeout(debounceRef.current);
      };
    }

    debounceRef.current = setTimeout(async () => {
      const controller = new AbortController();
      abortRef.current = controller;

      try {
        const data = await searchDocuments(
          vaultId,
          trimmed,
          10,
          controller.signal,
        );
        if (!controller.signal.aborted) {
          setResults(data);
          setError(null);
          setSearchedQuery(trimmed);
        }
      } catch (err) {
        if (!controller.signal.aborted) {
          console.error("Search failed:", err);
          setResults([]);
          setError(err instanceof Error ? err.message : t("error"));
          setSearchedQuery(trimmed);
        }
      }
    }, 300);

    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current);
    };
  }, [query, vaultId, t]);

  // Reset state when closing
  useEffect(() => {
    if (!open) {
      abortRef.current?.abort();
      if (debounceRef.current) clearTimeout(debounceRef.current);
      // Small delay so the transition finishes before resetting
      const id = setTimeout(() => {
        setQuery("");
        setResults([]);
        setSearchedQuery(null);
        setError(null);
      }, 200);
      return () => clearTimeout(id);
    }
  }, [open]);

  function handleSelect(value: SelectionValue | null) {
    if (!value) return;
    const hash = headingPathToHash(value.headingPath);
    router.push(`/docs/${value.path}${hash}`);
    onClose();
  }

  return (
    <Transition show={open} as={Fragment}>
      <HeadlessDialog onClose={onClose} className="relative z-50">
        {/* Backdrop */}
        <TransitionChild
          as={Fragment}
          enter="ease-out duration-200"
          enterFrom="opacity-0"
          enterTo="opacity-100"
          leave="ease-in duration-150"
          leaveFrom="opacity-100"
          leaveTo="opacity-0"
        >
          <div className="fixed inset-0 bg-black/50" aria-hidden="true" />
        </TransitionChild>

        {/* Panel — positioned near the top */}
        <div className="fixed inset-0 flex justify-center px-4 pt-[15vh]">
          <TransitionChild
            as={Fragment}
            enter="ease-out duration-200"
            enterFrom="opacity-0 scale-95"
            enterTo="opacity-100 scale-100"
            leave="ease-in duration-150"
            leaveFrom="opacity-100 scale-100"
            leaveTo="opacity-0 scale-95"
          >
            <DialogPanel className="w-full max-w-lg self-start">
              <Combobox<SelectionValue | null>
                onChange={handleSelect}
                value={null}
              >
                <div
                  className={cn(
                    "overflow-hidden rounded-2xl shadow-2xl",
                    "bg-white ring-1 ring-slate-200",
                    "dark:bg-slate-900 dark:ring-slate-800",
                  )}
                >
                  {/* Search input */}
                  <div className="flex items-center gap-3 px-4">
                    <MagnifyingGlassIcon className="size-5 shrink-0 text-slate-400" />
                    <ComboboxInput
                      className={cn(
                        "w-full border-0 bg-transparent py-3.5 text-sm",
                        "text-slate-900 placeholder:text-slate-400",
                        "dark:text-white dark:placeholder:text-slate-500",
                        "focus:outline-none",
                      )}
                      placeholder={t("placeholder")}
                      onChange={(e) => setQuery(e.target.value)}
                      autoFocus
                    />
                  </div>

                  {/* Results area */}
                  {(query.trim() !== "" || loading) && (
                    <>
                      <div className="border-t border-slate-200 dark:border-slate-800" />
                      <ResultsContent
                        loading={loading}
                        error={error}
                        results={results}
                        t={t}
                      />
                    </>
                  )}
                </div>
              </Combobox>
            </DialogPanel>
          </TransitionChild>
        </div>
      </HeadlessDialog>
    </Transition>
  );
}

export { SearchCommandPalette };
export type { SearchCommandPaletteProps };
