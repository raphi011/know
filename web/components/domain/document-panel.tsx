"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import {
  ListBulletIcon,
  ChevronDoubleRightIcon,
  InformationCircleIcon,
} from "@heroicons/react/20/solid";
import { Tabs } from "@/components/ui/tabs";
import { Sheet } from "@/components/ui/sheet";
import { ScrollArea } from "@/components/ui/scroll-area";
import { ErrorBoundary } from "@/components/ui/error-boundary";
import { DocumentOutline } from "@/components/domain/document-outline";
import { DocumentInfo } from "@/components/domain/document-info";
import type { Heading } from "@/app/lib/extract-headings";
import type { Document } from "@/app/lib/knowhow/types";
import { cn } from "@/lib/utils";

type DocumentPanelProps = {
  headings: Heading[];
  document: Document;
  scrollContainer: HTMLElement | null;
};

function DocumentPanel({
  headings,
  document,
  scrollContainer,
}: DocumentPanelProps) {
  const t = useTranslations("docs");
  const [collapsed, setCollapsed] = useState(false);
  const [sheetOpen, setSheetOpen] = useState(false);

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
  ];

  return (
    <>
      {/* Desktop panel (lg+) */}
      <div className="hidden lg:flex">
        {!collapsed && (
          <div className="flex w-64 shrink-0 flex-col border-l border-slate-200 dark:border-slate-700">
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
            "bg-primary-600 text-white hover:bg-primary-700",
            "dark:bg-primary-500 dark:hover:bg-primary-600",
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
