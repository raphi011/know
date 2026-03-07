"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import {
  MagnifyingGlassIcon,
  DocumentTextIcon,
  GlobeAltIcon,
  ChevronRightIcon,
} from "@heroicons/react/20/solid";
import { cn } from "@/lib/utils";
import type { ToolResultMeta } from "@/components/domain/agent-chat-context";

type ToolCardProps = {
  tool: string;
  callContent: string;
  result?: { content?: string; meta?: ToolResultMeta };
};

function ToolCard({ tool, callContent, result }: ToolCardProps) {
  const t = useTranslations("docs");
  const [expanded, setExpanded] = useState(false);
  const inflight = !result;
  const meta = result?.meta;
  const canExpand = !!meta;

  return (
    <div className="mb-0.5">
      {/* Single log line */}
      <button
        type="button"
        disabled={!canExpand}
        onClick={() => canExpand && setExpanded(!expanded)}
        className={cn(
          "group flex w-full items-center gap-1.5 py-0.5 text-left",
          "text-[11px] text-slate-400 dark:text-slate-500",
          canExpand && "cursor-pointer hover:text-slate-600 dark:hover:text-slate-300",
          inflight && "animate-pulse",
        )}
      >
        {/* Icon */}
        <span className="flex-shrink-0">
          {inflight ? <Spinner /> : toolIcon(tool)}
        </span>

        {/* Natural-language summary */}
        <span className="min-w-0 flex-1 truncate">
          {toolSummary(tool, callContent, meta)}
        </span>

        {/* Expand chevron */}
        {canExpand && (
          <ChevronRightIcon
            className={cn(
              "size-3 flex-shrink-0 transition-transform duration-150",
              expanded && "rotate-90",
            )}
          />
        )}
      </button>

      {/* Expandable detail */}
      {canExpand && (
        <div
          className={cn(
            "grid transition-all duration-200",
            expanded ? "grid-rows-[1fr]" : "grid-rows-[0fr]",
          )}
        >
          <div className="overflow-hidden">
            <div className="ml-5 space-y-0.5 pb-1">
              {/* Stats line */}
              <ExpandedStats t={t} tool={tool} meta={meta} />

              {/* Matched doc links */}
              {meta?.matchedDocs?.map((doc) => (
                <a
                  key={doc.path}
                  href={`/docs${doc.path}`}
                  className="flex items-center gap-1.5 text-[10px] text-primary-600 hover:underline dark:text-primary-400"
                >
                  <span className="min-w-0 truncate">{doc.title}</span>
                  <span className="flex-shrink-0 text-slate-400">
                    {(doc.score * 100).toFixed(0)}%
                  </span>
                </a>
              ))}

              {/* Web source links */}
              {meta?.webSources?.map((src) => (
                <a
                  key={src.url}
                  href={src.url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="block truncate text-[10px] text-primary-600 hover:underline dark:text-primary-400"
                >
                  {src.title}
                </a>
              ))}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

/** Build a natural-language summary for the log line. */
function toolSummary(
  tool: string,
  callContent: string,
  meta?: ToolResultMeta,
): string {
  if (tool === "kb_search") {
    return `Searched "${callContent}"`;
  }
  if (tool === "web_search") {
    return `Searched web for "${callContent}"`;
  }
  // read_document — prefer the resolved title from meta if available
  const label = meta?.documentTitle ?? callContent;
  if (!meta?.documentTitle && !meta?.contentLength) {
    return `Read ${label} — not found`;
  }
  return `Read ${label}`;
}

/** Plain-text stats shown in the expanded section. */
function ExpandedStats({
  t,
  tool,
  meta,
}: {
  t: ReturnType<typeof useTranslations<"docs">>;
  tool: string;
  meta: ToolResultMeta;
}) {
  const parts: string[] = [];

  if (tool === "kb_search") {
    if (meta.resultCount != null) parts.push(t("agentToolDocs", { count: meta.resultCount }));
    if (meta.chunkCount != null) parts.push(t("agentToolChunks", { count: meta.chunkCount }));
  }
  if (tool === "read_document" && meta.contentLength != null) {
    parts.push(t("agentToolSize", { size: Math.round(meta.contentLength / 1024) }));
  }
  if (tool === "web_search" && meta.webResultCount != null) {
    parts.push(t("agentToolWebResults", { count: meta.webResultCount }));
  }
  parts.push(t("agentToolDuration", { ms: meta.durationMs }));

  return (
    <span className="text-[10px] text-slate-400 dark:text-slate-500">
      {parts.join(" · ")}
    </span>
  );
}

function Spinner() {
  return (
    <svg
      className="size-3.5 animate-spin text-slate-400"
      xmlns="http://www.w3.org/2000/svg"
      fill="none"
      viewBox="0 0 24 24"
    >
      <circle
        className="opacity-25"
        cx="12"
        cy="12"
        r="10"
        stroke="currentColor"
        strokeWidth="4"
      />
      <path
        className="opacity-75"
        fill="currentColor"
        d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
      />
    </svg>
  );
}

function toolIcon(tool: string) {
  if (tool === "kb_search")
    return <MagnifyingGlassIcon className="size-3.5" />;
  if (tool === "web_search") return <GlobeAltIcon className="size-3.5" />;
  return <DocumentTextIcon className="size-3.5" />;
}

export { ToolCard };
export type { ToolCardProps };
