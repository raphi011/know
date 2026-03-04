"use client";

import { useState, useCallback, useRef } from "react";
import { useTranslations } from "next-intl";
import {
  ListBulletIcon,
  ChevronDoubleRightIcon,
  InformationCircleIcon,
  ChatBubbleLeftRightIcon,
} from "@heroicons/react/20/solid";
import { Tabs } from "@/components/ui/tabs";
import { Sheet } from "@/components/ui/sheet";
import { ScrollArea } from "@/components/ui/scroll-area";
import { ErrorBoundary } from "@/components/ui/error-boundary";
import { DocumentOutline } from "@/components/domain/document-outline";
import { DocumentInfo } from "@/components/domain/document-info";
import { DocumentHistory } from "@/components/domain/document-history";
import { AgentChatPanel } from "@/components/domain/agent-chat-panel";
import type { Heading } from "@/app/lib/extract-headings";
import type { Document, DocumentVersion } from "@/app/lib/knowhow/types";
import { cn } from "@/lib/utils";

const PANEL_STORAGE_KEY = "kh_panel_width";
const PANEL_MIN_WIDTH = 256;
const PANEL_DEFAULT_WIDTH = 256;
const PANEL_MAX_WIDTH_RATIO = 0.5;

function getStoredWidth(): number {
  if (typeof window === "undefined") return PANEL_DEFAULT_WIDTH;
  const stored = localStorage.getItem(PANEL_STORAGE_KEY);
  if (!stored) return PANEL_DEFAULT_WIDTH;
  const parsed = Number(stored);
  return Number.isFinite(parsed) ? Math.max(PANEL_MIN_WIDTH, parsed) : PANEL_DEFAULT_WIDTH;
}

type DocumentPanelProps = {
  headings: Heading[];
  document: Document;
  scrollContainer: HTMLElement | null;
  vaultId: string | null;
  versions: DocumentVersion[];
  versionsTotalCount: number;
};

function DocumentPanel({
  headings,
  document,
  scrollContainer,
  vaultId,
  versions,
  versionsTotalCount,
}: DocumentPanelProps) {
  const t = useTranslations("docs");
  const [collapsed, setCollapsed] = useState(false);
  const [sheetOpen, setSheetOpen] = useState(false);
  const [panelWidth, setPanelWidth] = useState(getStoredWidth);
  const isDragging = useRef(false);
  const panelRef = useRef<HTMLDivElement>(null);

  const onPointerDown = useCallback((e: React.PointerEvent) => {
    e.preventDefault();
    isDragging.current = true;
    globalThis.document.body.style.cursor = "col-resize";
    globalThis.document.body.style.userSelect = "none";

    const onPointerMove = (ev: PointerEvent) => {
      if (!isDragging.current || !panelRef.current) return;
      // Use the parent flex container (preview + panel) to calculate max width
      const parent = panelRef.current.parentElement;
      if (!parent) return;
      const parentRect = parent.getBoundingClientRect();
      const maxWidth = parentRect.width * PANEL_MAX_WIDTH_RATIO;
      const newWidth = Math.min(maxWidth, Math.max(PANEL_MIN_WIDTH, parentRect.right - ev.clientX));
      setPanelWidth(newWidth);
    };

    const onPointerUp = () => {
      isDragging.current = false;
      globalThis.document.body.style.cursor = "";
      globalThis.document.body.style.userSelect = "";
      window.removeEventListener("pointermove", onPointerMove);
      window.removeEventListener("pointerup", onPointerUp);
      // Persist final width
      setPanelWidth((w) => {
        localStorage.setItem(PANEL_STORAGE_KEY, String(Math.round(w)));
        return w;
      });
    };

    window.addEventListener("pointermove", onPointerMove);
    window.addEventListener("pointerup", onPointerUp);
  }, []);

  const tabItems = [
    {
      label: t("outline"),
      content: (
        <DocumentOutline
          headings={headings}
          scrollContainer={scrollContainer}
        />
      ),
    },
    {
      label: t("info"),
      content: <DocumentInfo document={document} />,
    },
    {
      label: t("history"),
      content: (
        <DocumentHistory
          documentId={document.id}
          vaultId={vaultId ?? ""}
          versions={versions}
          totalCount={versionsTotalCount}
        />
      ),
    },
    {
      label: t("agent"),
      content: <AgentChatPanel vaultId={vaultId} />,
    },
  ];

  return (
    <>
      {/* Desktop panel (lg+) */}
      <div className="hidden lg:flex" ref={panelRef}>
        {!collapsed && (
          <div
            className="relative flex shrink-0 flex-col border-l border-slate-200 dark:border-slate-700"
            style={{ width: panelWidth }}
          >
            {/* Resize drag handle */}
            <div
              onPointerDown={onPointerDown}
              className="absolute inset-y-0 -left-1 z-10 w-2 cursor-col-resize hover:bg-primary-400/30 active:bg-primary-400/50 transition-colors"
            />
            <div className="flex items-center justify-end px-2 py-1">
              <button
                type="button"
                onClick={() => setCollapsed(true)}
                className="rounded-md p-1 text-slate-400 hover:bg-slate-100 hover:text-slate-600 dark:hover:bg-slate-800 dark:hover:text-slate-300"
                title={t("collapsePanel")}
              >
                <ChevronDoubleRightIcon className="size-4" />
              </button>
            </div>
            <ScrollArea className="flex-1 px-3 pb-4">
              <ErrorBoundary
                fallback={
                  <p className="px-2 text-sm text-red-500">
                    {t("panelError")}
                  </p>
                }
              >
                <Tabs items={tabItems} />
              </ErrorBoundary>
            </ScrollArea>
          </div>
        )}
        {collapsed && (
          <div className="flex shrink-0 flex-col items-center border-l border-slate-200 px-1 py-2 dark:border-slate-700">
            <button
              type="button"
              onClick={() => setCollapsed(false)}
              className="rounded-md p-1.5 text-slate-400 hover:bg-slate-100 hover:text-slate-600 dark:hover:bg-slate-800 dark:hover:text-slate-300"
              title={t("expandPanel")}
            >
              <ListBulletIcon className="size-4" />
            </button>
          </div>
        )}
      </div>

      {/* Mobile floating button + bottom sheet (<lg) */}
      <div className="lg:hidden">
        <button
          type="button"
          onClick={() => setSheetOpen(true)}
          className={cn(
            "fixed bottom-4 right-4 z-40 flex items-center gap-1.5 rounded-full px-3.5 py-2 text-sm font-medium shadow-lg",
            "bg-white/90 text-slate-600 ring-1 ring-slate-200 backdrop-blur-sm hover:bg-slate-50",
            "dark:bg-slate-800/90 dark:text-slate-300 dark:ring-slate-700 dark:hover:bg-slate-700",
          )}
        >
          <InformationCircleIcon className="size-4" />
          {t("outline")}
        </button>
        <Sheet open={sheetOpen} onClose={() => setSheetOpen(false)}>
          <div className="max-h-[60vh] overflow-y-auto">
            <ErrorBoundary
              fallback={
                <p className="px-2 text-sm text-red-500">
                  {t("panelError")}
                </p>
              }
            >
              <Tabs items={tabItems} />
            </ErrorBoundary>
          </div>
        </Sheet>
      </div>
    </>
  );
}

export { DocumentPanel };
