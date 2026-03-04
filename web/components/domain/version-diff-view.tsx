"use client";

import { useState, useEffect } from "react";
import { useTranslations } from "next-intl";
import type { VersionDiff, DiffHunk, DiffLine } from "@/app/lib/knowhow/types";
import { cn } from "@/lib/utils";

type VersionDiffViewProps = {
  documentId: string;
  fromVersionId: string;
  toVersionId?: string;
};

function VersionDiffView({
  documentId,
  fromVersionId,
  toVersionId,
}: VersionDiffViewProps) {
  const t = useTranslations("docs");
  const [diff, setDiff] = useState<VersionDiff | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    setLoading(true);
    setError(null);
    setDiff(null);

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
    })
      .then(async (res) => {
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const json = await res.json();
        if (json.errors?.length) throw new Error(json.errors[0].message);
        setDiff(json.data.versionDiff);
      })
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  }, [documentId, fromVersionId, toVersionId]);

  if (loading) {
    return (
      <p className="px-1 text-xs text-slate-400 dark:text-slate-500">
        {t("diffLoading")}
      </p>
    );
  }

  if (error) {
    return (
      <p className="px-1 text-xs text-red-500">{t("diffError")}</p>
    );
  }

  if (!diff || diff.hunks.length === 0) {
    return null;
  }

  return (
    <div className="space-y-2">
      {/* Stats summary */}
      <div className="flex gap-3 px-1 text-xs">
        <span className="text-green-600 dark:text-green-400">
          {t("additions", { count: diff.stats.additions })}
        </span>
        <span className="text-red-600 dark:text-red-400">
          {t("deletions", { count: diff.stats.deletions })}
        </span>
      </div>

      {/* Hunks */}
      <div className="overflow-hidden rounded-lg border border-slate-200 dark:border-slate-700">
        {diff.hunks.map((hunk) => (
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
