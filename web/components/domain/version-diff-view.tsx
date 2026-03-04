"use client";

import { useState, useEffect, useRef } from "react";
import { useTranslations } from "next-intl";
import type { VersionDiff, DiffHunk, DiffLine } from "@/app/lib/knowhow/types";
import { cn } from "@/lib/utils";

type VersionDiffViewProps = {
  documentId: string;
  fromVersionId: string;
  toVersionId?: string;
};

type DiffState =
  | { status: "loading" }
  | { status: "error"; message: string }
  | { status: "done"; diff: VersionDiff };

function VersionDiffView({
  documentId,
  fromVersionId,
  toVersionId,
}: VersionDiffViewProps) {
  const t = useTranslations("docs");
  const [state, setState] = useState<DiffState>({ status: "loading" });
  const abortRef = useRef<AbortController | null>(null);

  useEffect(() => {
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;

    const query = `query ($documentId: ID!, $fromVersionId: ID, $toVersionId: ID) {
      versionDiff(documentId: $documentId, fromVersionId: $fromVersionId, toVersionId: $toVersionId) {
        hunks {
          index oldStart oldLines newStart newLines header
          lines { type content oldLineNo newLineNo }
        }
        hasConflict
        stats { additions deletions hunksCount }
      }
    }`;

    fetch("/api/graphql", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        query,
        variables: { documentId, fromVersionId, toVersionId: toVersionId ?? null },
      }),
      signal: controller.signal,
    })
      .then(async (res) => {
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const json = await res.json();
        if (json.errors?.length) throw new Error(json.errors[0].message);
        setState({ status: "done", diff: json.data.versionDiff });
      })
      .catch((err) => {
        if (err instanceof DOMException && err.name === "AbortError") return;
        setState({ status: "error", message: err.message });
      });

    return () => controller.abort();
  }, [documentId, fromVersionId, toVersionId]);

  if (state.status === "loading") {
    return (
      <p className="px-1 text-xs text-slate-400 dark:text-slate-500">
        {t("diffLoading")}
      </p>
    );
  }

  if (state.status === "error") {
    return (
      <p className="px-1 text-xs text-red-500">
        {t("diffError")}: {state.message}
      </p>
    );
  }

  if (state.diff.hunks.length === 0) {
    return (
      <p className="px-1 text-xs text-slate-400 dark:text-slate-500">
        {t("noChanges")}
      </p>
    );
  }

  return (
    <div className="space-y-2">
      {/* Stats summary */}
      <div className="flex gap-3 px-1 text-xs">
        <span className="text-green-600 dark:text-green-400">
          {t("additions", { count: state.diff.stats.additions })}
        </span>
        <span className="text-red-600 dark:text-red-400">
          {t("deletions", { count: state.diff.stats.deletions })}
        </span>
      </div>

      {/* Hunks */}
      <div className="overflow-hidden rounded-lg border border-slate-200 dark:border-slate-700">
        {state.diff.hunks.map((hunk) => (
          <HunkView key={hunk.index} hunk={hunk} />
        ))}
      </div>
    </div>
  );
}

function HunkView({ hunk }: { hunk: DiffHunk }) {
  return (
    <div>
      <div className="bg-slate-100 px-3 py-1 text-xs font-mono text-slate-500 dark:bg-slate-800 dark:text-slate-400">
        {hunk.header}
      </div>
      <div className="font-mono text-xs leading-5">
        {hunk.lines.map((line, i) => (
          <LineView key={i} line={line} />
        ))}
      </div>
    </div>
  );
}

function LineView({ line }: { line: DiffLine }) {
  const prefix =
    line.type === "ADD" ? "+" : line.type === "DELETE" ? "-" : " ";

  return (
    <div
      className={cn(
        "flex",
        line.type === "ADD" &&
          "bg-green-50 text-green-800 dark:bg-green-950/40 dark:text-green-300",
        line.type === "DELETE" &&
          "bg-red-50 text-red-800 dark:bg-red-950/40 dark:text-red-300",
        line.type === "CONTEXT" &&
          "text-slate-600 dark:text-slate-400",
      )}
    >
      <span className="w-8 shrink-0 select-none text-right text-slate-400 dark:text-slate-600 pr-1">
        {line.oldLineNo ?? ""}
      </span>
      <span className="w-8 shrink-0 select-none text-right text-slate-400 dark:text-slate-600 pr-1">
        {line.newLineNo ?? ""}
      </span>
      <span className="w-4 shrink-0 select-none text-center">{prefix}</span>
      <span className="min-w-0 whitespace-pre-wrap break-all pr-2">
        {line.content}
      </span>
    </div>
  );
}

export { VersionDiffView };
