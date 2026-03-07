"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import {
  MagnifyingGlassIcon,
  DocumentTextIcon,
  GlobeAltIcon,
  ChevronDownIcon,
} from "@heroicons/react/20/solid";
import { cn } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
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
  const hasExpandableContent =
    (meta?.matchedDocs && meta.matchedDocs.length > 0) ||
    (meta?.webSources && meta.webSources.length > 0);

  return (
    <div
      className={cn(
        "mb-2 rounded-xl border px-3 py-2",
        "border-slate-200 bg-white dark:border-slate-700 dark:bg-slate-900",
        inflight && "animate-pulse",
      )}
    >
      {/* Header row */}
      <div className="flex items-center gap-2">
        {/* Icon */}
        <span className="flex-shrink-0 text-slate-400 dark:text-slate-500">
          {inflight ? <Spinner /> : toolIcon(tool)}
        </span>

        {/* Tool label */}
        <span className="text-[11px] font-medium text-slate-700 dark:text-slate-300">
          {toolLabel(t, tool)}
        </span>

        {/* Query/path text */}
        <span className="min-w-0 flex-1 truncate text-[11px] text-slate-500 dark:text-slate-400">
          {callContent}
        </span>

        {/* Metadata badges */}
        {meta && (
          <div className="flex flex-shrink-0 items-center gap-1">
            <MetaBadges t={t} tool={tool} meta={meta} />
          </div>
        )}

        {/* Expand chevron */}
        {hasExpandableContent && (
          <button
            type="button"
            onClick={() => setExpanded(!expanded)}
            className="flex-shrink-0 rounded p-0.5 text-slate-400 hover:bg-slate-100 hover:text-slate-600 dark:hover:bg-slate-800 dark:hover:text-slate-300"
          >
            <ChevronDownIcon
              className={cn(
                "size-3.5 transition-transform duration-200",
                expanded && "rotate-180",
              )}
            />
          </button>
        )}
      </div>

      {/* Expandable detail section */}
      {hasExpandableContent && (
        <div
          className={cn(
            "grid transition-all duration-200 motion-safe:transition-all",
            expanded ? "grid-rows-[1fr]" : "grid-rows-[0fr]",
          )}
        >
          <div className="overflow-hidden">
            <div className="pt-2">
              {meta?.matchedDocs && meta.matchedDocs.length > 0 && (
                <ul className="space-y-0.5">
                  {meta.matchedDocs.map((doc) => (
                    <li key={doc.path} className="flex items-center gap-1.5">
                      <a
                        href={`/docs${doc.path}`}
                        className="min-w-0 truncate text-[10px] text-primary-600 hover:underline dark:text-primary-400"
                      >
                        {doc.title}
                      </a>
                      <span className="flex-shrink-0 text-[9px] text-slate-400">
                        {(doc.score * 100).toFixed(0)}%
                      </span>
                    </li>
                  ))}
                </ul>
              )}
              {meta?.webSources && meta.webSources.length > 0 && (
                <ul className="space-y-0.5">
                  {meta.webSources.map((src) => (
                    <li key={src.url}>
                      <a
                        href={src.url}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="truncate text-[10px] text-primary-600 hover:underline dark:text-primary-400"
                      >
                        {src.title}
                      </a>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
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

function toolLabel(
  t: ReturnType<typeof useTranslations<"docs">>,
  tool: string,
) {
  if (tool === "kb_search") return t("agentToolKbSearch");
  if (tool === "web_search") return t("agentToolWebSearch");
  return t("agentToolReadDoc");
}

function MetaBadges({
  t,
  tool,
  meta,
}: {
  t: ReturnType<typeof useTranslations<"docs">>;
  tool: string;
  meta: ToolResultMeta;
}) {
  return (
    <>
      {tool === "kb_search" && meta.resultCount != null && (
        <Badge variant="subtle" size="sm">
          {t("agentToolDocs", { count: meta.resultCount })}
        </Badge>
      )}
      {tool === "kb_search" && meta.chunkCount != null && (
        <Badge variant="subtle" size="sm">
          {t("agentToolChunks", { count: meta.chunkCount })}
        </Badge>
      )}
      {tool === "read_document" && meta.documentTitle && (
        <Badge variant="subtle" size="sm">
          {meta.documentTitle}
        </Badge>
      )}
      {tool === "read_document" && meta.contentLength != null && (
        <Badge variant="subtle" size="sm">
          {t("agentToolSize", {
            size: Math.round(meta.contentLength / 1024),
          })}
        </Badge>
      )}
      {tool === "web_search" && meta.webResultCount != null && (
        <Badge variant="subtle" size="sm">
          {t("agentToolWebResults", { count: meta.webResultCount })}
        </Badge>
      )}
      {tool === "read_document" &&
        !meta.documentTitle &&
        !meta.contentLength && (
          <Badge variant="warning" size="sm">
            {t("agentToolNotFound")}
          </Badge>
        )}
      <Badge variant="subtle" size="sm">
        {t("agentToolDuration", { ms: meta.durationMs })}
      </Badge>
    </>
  );
}

export { ToolCard };
export type { ToolCardProps };
