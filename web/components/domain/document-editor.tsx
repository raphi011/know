"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { PencilSquareIcon, EyeIcon } from "@heroicons/react/20/solid";
import { Badge } from "@/components/ui/badge";
import { MarkdownEditor } from "@/components/domain/markdown-editor";
import { MarkdownRenderer } from "@/components/domain/markdown-renderer";
import { saveDocument } from "@/app/lib/knowhow/mutations";
import type { Document } from "@/app/lib/knowhow/types";
import { cn } from "@/lib/utils";

type SaveStatus = "idle" | "unsaved" | "saving" | "saved" | "error";

type DocumentEditorProps = {
  document: Document;
  vaultId: string;
};

const SAVE_DELAY_MS = 1500;

type Mode = "edit" | "preview";

function DocumentEditor({ document, vaultId }: DocumentEditorProps) {
  const [status, setStatus] = useState<SaveStatus>("idle");
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [mode, setMode] = useState<Mode>("preview");
  const [previewContent, setPreviewContent] = useState(document.content);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const contentRef = useRef(document.content);
  const t = useTranslations("docs");

  useEffect(() => {
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current);
    };
  }, []);

  function handleModeChange(next: Mode) {
    if (next === "preview") {
      setPreviewContent(contentRef.current);
    }
    setMode(next);
  }

  function handleChange(content: string) {
    contentRef.current = content;
    setStatus("unsaved");
    setErrorMessage(null);

    if (timerRef.current) {
      clearTimeout(timerRef.current);
    }

    timerRef.current = setTimeout(async () => {
      setStatus("saving");
      const result = await saveDocument(
        vaultId,
        document.path,
        contentRef.current,
      );
      if (result.success) {
        setStatus("saved");
        setErrorMessage(null);
      } else {
        setStatus("error");
        setErrorMessage(result.error);
        console.error("Document save failed:", result.error);
      }
    }, SAVE_DELAY_MS);
  }

  return (
    <div className="flex h-[calc(100vh-5rem)] flex-col gap-3 lg:h-[calc(100vh-3rem)]">
      {/* Toolbar */}
      <div className="flex items-center gap-3">
        <h1 className="text-lg font-semibold text-slate-900 dark:text-white">
          {document.title}
        </h1>
        {document.labels.map((label) => (
          <Badge key={label} variant="subtle">
            {label}
          </Badge>
        ))}
        <div className="ml-auto flex items-center gap-3">
          <StatusIndicator status={status} errorMessage={errorMessage} t={t} />
          <ModeToggle mode={mode} onModeChange={handleModeChange} t={t} />
        </div>
      </div>

      {/* Editor (kept mounted to preserve undo/scroll state) */}
      <div className={cn("min-h-0 flex-1", mode !== "edit" && "hidden")}>
        <MarkdownEditor content={document.content} onChange={handleChange} />
      </div>

      {/* Preview */}
      {mode === "preview" && (
        <div className="min-h-0 flex-1 overflow-y-auto">
          <MarkdownRenderer content={previewContent} />
        </div>
      )}
    </div>
  );
}

function ModeToggle({
  mode,
  onModeChange,
  t,
}: {
  mode: Mode;
  onModeChange: (mode: Mode) => void;
  t: ReturnType<typeof useTranslations<"docs">>;
}) {
  return (
    <div className="flex rounded-lg bg-slate-100 p-0.5 dark:bg-slate-800">
      <button
        type="button"
        onClick={() => onModeChange("edit")}
        className={cn(
          "flex items-center gap-1.5 rounded-md px-2.5 py-1 text-sm font-medium transition-colors",
          mode === "edit"
            ? "bg-white text-slate-900 shadow-sm dark:bg-slate-700 dark:text-white"
            : "text-slate-600 hover:text-slate-900 dark:text-slate-400 dark:hover:text-white",
        )}
      >
        <PencilSquareIcon className="size-4" />
        {t("edit")}
      </button>
      <button
        type="button"
        onClick={() => onModeChange("preview")}
        className={cn(
          "flex items-center gap-1.5 rounded-md px-2.5 py-1 text-sm font-medium transition-colors",
          mode === "preview"
            ? "bg-white text-slate-900 shadow-sm dark:bg-slate-700 dark:text-white"
            : "text-slate-600 hover:text-slate-900 dark:text-slate-400 dark:hover:text-white",
        )}
      >
        <EyeIcon className="size-4" />
        {t("preview")}
      </button>
    </div>
  );
}

function StatusIndicator({
  status,
  errorMessage,
  t,
}: {
  status: SaveStatus;
  errorMessage: string | null;
  t: ReturnType<typeof useTranslations<"docs">>;
}) {
  if (status === "idle") return null;

  const config = {
    unsaved: {
      label: t("unsaved"),
      color: "text-amber-600 dark:text-amber-400",
    },
    saving: { label: t("saving"), color: "text-slate-500 dark:text-slate-400" },
    saved: { label: t("saved"), color: "text-green-600 dark:text-green-400" },
    error: { label: t("saveError"), color: "text-red-600 dark:text-red-400" },
  } as const;

  const { label, color } = config[status];

  return (
    <span
      className={cn("text-sm", color)}
      title={status === "error" && errorMessage ? errorMessage : undefined}
    >
      {label}
    </span>
  );
}

export { DocumentEditor };
