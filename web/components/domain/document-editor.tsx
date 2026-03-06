"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { PencilSquareIcon, EyeIcon } from "@heroicons/react/20/solid";
import { Badge } from "@/components/ui/badge";
import { MarkdownEditor } from "@/components/domain/markdown-editor";
import { MarkdownRenderer } from "@/components/domain/markdown-renderer";
import { DocumentPanel } from "@/components/domain/document-panel";
import { useShowToolbarTitle } from "@/hooks/use-h1-visibility";
import { saveDocument } from "@/app/lib/knowhow/mutations";
import { extractHeadings } from "@/app/lib/extract-headings";
import type { Document, DocumentVersion } from "@/app/lib/knowhow/types";
import { cn } from "@/lib/utils";

type SaveStatus = "idle" | "unsaved" | "saving" | "saved" | "error";

type DocumentEditorProps = {
  document: Document;
  vaultId: string;
  versions: DocumentVersion[];
  versionsTotalCount: number;
};

const SAVE_DELAY_MS = 1500;

type Mode = "edit" | "preview";

function DocumentEditor({
  document,
  vaultId,
  versions,
  versionsTotalCount,
}: DocumentEditorProps) {
  const [status, setStatus] = useState<SaveStatus>("idle");
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [mode, setMode] = useState<Mode>("preview");
  const [previewContent, setPreviewContent] = useState(document.content);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const contentRef = useRef(document.content);
  const t = useTranslations("docs");

  const showToolbarTitle = useShowToolbarTitle(mode === "preview");
  const headings = extractHeadings(previewContent);

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
    <div className="flex flex-col gap-3">
      {/* Toolbar — sticky, spans full width including panel area */}
      <div className="sticky top-[3.75rem] z-10 -mx-4 flex items-center gap-3 bg-slate-50/95 px-4 py-2 backdrop-blur-sm lg:top-0 lg:-mx-6 lg:px-6 dark:bg-slate-950/95">
        <h1
          className={cn(
            "min-w-0 truncate text-lg font-semibold text-slate-900 transition-opacity duration-200 dark:text-white",
            showToolbarTitle ? "opacity-100" : "opacity-0",
          )}
          aria-hidden={!showToolbarTitle}
        >
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

      {/* Preview + Panel — body scrolls the preview, panel is sticky */}
      {mode === "preview" && (
        <div className="flex items-start">
          <div className="min-w-0 flex-1">
            <MarkdownRenderer content={previewContent} />
          </div>
          <DocumentPanel
            headings={headings}
            document={document}
            vaultId={vaultId}
            versions={versions}
            versionsTotalCount={versionsTotalCount}
          />
        </div>
      )}
    </div>
  );
}

const modeOptions = [
  { value: "edit" as const, icon: PencilSquareIcon },
  { value: "preview" as const, icon: EyeIcon },
];

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
      {modeOptions.map(({ value, icon: Icon }) => (
        <button
          key={value}
          type="button"
          onClick={() => onModeChange(value)}
          aria-label={t(value)}
          className={cn(
            "flex items-center gap-1.5 rounded-md px-2.5 py-1 text-sm font-medium transition-colors",
            mode === value
              ? "bg-white text-slate-900 shadow-sm dark:bg-slate-700 dark:text-white"
              : "text-slate-600 hover:text-slate-900 dark:text-slate-400 dark:hover:text-white",
          )}
        >
          <Icon className="size-4" />
          <span className="hidden sm:inline">{t(value)}</span>
        </button>
      ))}
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
      className={cn("max-w-48 truncate text-sm", color)}
      title={status === "error" && errorMessage ? errorMessage : undefined}
    >
      {status === "error" && errorMessage ? errorMessage : label}
    </span>
  );
}

export { DocumentEditor };
