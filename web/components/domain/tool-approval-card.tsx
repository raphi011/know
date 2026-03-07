"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { useAgentChat } from "./agent-chat-context";
import type { PendingApproval } from "./agent-chat-reducer";
import { cn } from "@/lib/utils";

type Props = { approval: PendingApproval };

export function ToolApprovalCard({ approval }: Props) {
  const t = useTranslations("docs");
  const { respondToApproval } = useAgentChat();
  const [selectedHunks, setSelectedHunks] = useState<Set<number>>(new Set());
  const [isSubmitting, setIsSubmitting] = useState(false);

  const handleApproveAll = async () => {
    setIsSubmitting(true);
    await respondToApproval("approve_all");
  };

  const handleApproveSelected = async () => {
    setIsSubmitting(true);
    await respondToApproval("approve_hunks", Array.from(selectedHunks));
  };

  const handleReject = async () => {
    setIsSubmitting(true);
    await respondToApproval("reject");
  };

  const toggleHunk = (index: number) => {
    setSelectedHunks((prev) => {
      const next = new Set(prev);
      if (next.has(index)) next.delete(index);
      else next.add(index);
      return next;
    });
  };

  const toolLabel =
    approval.tool === "create_document"
      ? t("approvalNewDoc")
      : t("approvalEditDoc");

  return (
    <div className="mb-2 rounded-lg border border-amber-300 bg-amber-50/50 dark:border-amber-700 dark:bg-amber-950/30">
      {/* Header */}
      <div className="flex items-center gap-2 border-b border-amber-200 px-3 py-2 dark:border-amber-800">
        <span className="text-xs font-medium text-amber-700 dark:text-amber-300">
          {toolLabel}
        </span>
        <span className="truncate font-mono text-xs text-slate-500 dark:text-slate-400">
          {approval.path}
        </span>
      </div>

      {/* Content */}
      <div className="max-h-[300px] overflow-y-auto">
        {approval.isNew && approval.content ? (
          <pre className="whitespace-pre-wrap break-all bg-green-50/50 p-3 font-mono text-[11px] leading-5 text-green-800 dark:bg-green-950/20 dark:text-green-300">
            {approval.content}
          </pre>
        ) : approval.diff ? (
          <div>
            {approval.diff.hunks.map((hunk) => (
              <div key={hunk.index}>
                <div className="flex items-center gap-2 bg-slate-100 px-3 py-1 dark:bg-slate-800">
                  <input
                    type="checkbox"
                    checked={selectedHunks.has(hunk.index)}
                    onChange={() => toggleHunk(hunk.index)}
                    disabled={isSubmitting}
                    className="size-3 rounded border-slate-300 text-primary-600"
                  />
                  <span className="font-mono text-[10px] text-slate-500 dark:text-slate-400">
                    @@ -{hunk.old_start},{hunk.old_lines} +{hunk.new_start},
                    {hunk.new_lines} @@
                  </span>
                </div>
                <div className="font-mono text-[11px] leading-5">
                  {hunk.lines.map((line, i) => {
                    const prefix =
                      line.type === "add"
                        ? "+"
                        : line.type === "delete"
                          ? "-"
                          : " ";
                    return (
                      <div
                        key={i}
                        className={cn(
                          "flex",
                          line.type === "add" &&
                            "bg-green-50 text-green-800 dark:bg-green-950/40 dark:text-green-300",
                          line.type === "delete" &&
                            "bg-red-50 text-red-800 dark:bg-red-950/40 dark:text-red-300",
                          line.type === "context" &&
                            "text-slate-600 dark:text-slate-400",
                        )}
                      >
                        <span className="w-8 shrink-0 select-none pr-1 text-right text-slate-400 dark:text-slate-600">
                          {line.old_line_no ?? ""}
                        </span>
                        <span className="w-8 shrink-0 select-none pr-1 text-right text-slate-400 dark:text-slate-600">
                          {line.new_line_no ?? ""}
                        </span>
                        <span className="w-4 shrink-0 select-none text-center">
                          {prefix}
                        </span>
                        <span className="min-w-0 whitespace-pre-wrap break-all pr-2">
                          {line.content}
                        </span>
                      </div>
                    );
                  })}
                </div>
              </div>
            ))}
          </div>
        ) : null}
      </div>

      {/* Stats + Actions */}
      <div className="flex items-center justify-between border-t border-amber-200 px-3 py-2 dark:border-amber-800">
        <div className="flex gap-3 text-[10px]">
          {approval.diff && (
            <>
              <span className="text-green-600 dark:text-green-400">
                +{approval.diff.stats.additions}
              </span>
              <span className="text-red-600 dark:text-red-400">
                -{approval.diff.stats.deletions}
              </span>
            </>
          )}
        </div>
        <div className="flex gap-1.5">
          <button
            type="button"
            onClick={handleReject}
            disabled={isSubmitting}
            className="rounded px-2 py-1 text-[10px] font-medium text-red-600 hover:bg-red-50 disabled:opacity-50 dark:text-red-400 dark:hover:bg-red-950/50"
          >
            {t("approvalReject")}
          </button>
          {approval.diff && approval.diff.hunks.length > 1 && (
            <button
              type="button"
              onClick={handleApproveSelected}
              disabled={isSubmitting || selectedHunks.size === 0}
              className="rounded px-2 py-1 text-[10px] font-medium text-slate-600 hover:bg-slate-100 disabled:opacity-50 dark:text-slate-400 dark:hover:bg-slate-800"
            >
              {t("approvalApproveSelected")}
            </button>
          )}
          <button
            type="button"
            onClick={handleApproveAll}
            disabled={isSubmitting}
            className="rounded bg-primary-600 px-2 py-1 text-[10px] font-medium text-white hover:bg-primary-700 disabled:opacity-50"
          >
            {t("approvalApproveAll")}
          </button>
        </div>
      </div>
    </div>
  );
}
