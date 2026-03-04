"use client";

import { useState } from "react";
import { useTranslations, useFormatter } from "next-intl";
import { useRouter } from "next/navigation";
import { ArrowUturnLeftIcon } from "@heroicons/react/20/solid";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { VersionDiffView } from "@/components/domain/version-diff-view";
import { rollbackDocument } from "@/app/lib/knowhow/mutations";
import type { DocumentVersion } from "@/app/lib/knowhow/types";
import { cn } from "@/lib/utils";

type DocumentHistoryProps = {
  documentId: string;
  vaultId: string;
  versions: DocumentVersion[];
  totalCount: number;
};

function DocumentHistory({
  documentId,
  vaultId,
  versions,
  totalCount,
}: DocumentHistoryProps) {
  const t = useTranslations("docs");
  const tc = useTranslations("common");
  const format = useFormatter();
  const router = useRouter();
  const [selectedVersion, setSelectedVersion] = useState<DocumentVersion | null>(
    null,
  );
  const [rollbackTarget, setRollbackTarget] = useState<DocumentVersion | null>(
    null,
  );
  const [rolling, setRolling] = useState(false);

  async function handleRollback() {
    if (!rollbackTarget) return;
    setRolling(true);
    const result = await rollbackDocument(vaultId, documentId, rollbackTarget.id);
    setRolling(false);
    setRollbackTarget(null);
    if (result.success) {
      router.refresh();
    }
  }

  if (versions.length === 0) {
    return (
      <p className="text-sm text-slate-400 dark:text-slate-500">
        {t("noVersions")}
      </p>
    );
  }

  return (
    <div className="space-y-1">
      {/* Version list */}
      <div className="space-y-1">
        {versions.map((v) => (
          <button
            key={v.id}
            type="button"
            onClick={() =>
              setSelectedVersion(selectedVersion?.id === v.id ? null : v)
            }
            className={cn(
              "w-full rounded-lg px-2.5 py-2 text-left text-sm transition-colors",
              selectedVersion?.id === v.id
                ? "bg-primary-50 ring-1 ring-primary-200 dark:bg-primary-950 dark:ring-primary-800"
                : "hover:bg-slate-50 dark:hover:bg-slate-800/50",
            )}
          >
            <div className="flex items-center justify-between">
              <span className="font-medium text-slate-700 dark:text-slate-300">
                v{v.version}
              </span>
              <span className="text-xs text-slate-400 dark:text-slate-500">
                {format.dateTime(new Date(v.createdAt), {
                  dateStyle: "short",
                  timeStyle: "short",
                })}
              </span>
            </div>
            <p className="mt-0.5 truncate text-xs text-slate-500 dark:text-slate-400">
              {v.title}
            </p>
          </button>
        ))}
      </div>

      {totalCount > versions.length && (
        <p className="px-2.5 text-xs text-slate-400 dark:text-slate-500">
          +{totalCount - versions.length} more
        </p>
      )}

      {/* Expanded version: diff + rollback */}
      {selectedVersion && (
        <div className="mt-3 space-y-3">
          <div className="flex items-center justify-between px-1">
            <span className="text-xs font-medium text-slate-500 dark:text-slate-400">
              v{selectedVersion.version} &rarr; {t("currentVersion")}
            </span>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setRollbackTarget(selectedVersion)}
            >
              <ArrowUturnLeftIcon className="size-3.5" />
              {t("rollback")}
            </Button>
          </div>

          <VersionDiffView
            documentId={documentId}
            fromVersionId={selectedVersion.id}
          />
        </div>
      )}

      {/* Rollback confirmation dialog */}
      <Dialog
        open={rollbackTarget !== null}
        onClose={() => setRollbackTarget(null)}
        title={
          rollbackTarget
            ? t("rollbackConfirmTitle", { number: rollbackTarget.version })
            : ""
        }
      >
        <p className="mb-6 text-sm text-slate-600 dark:text-slate-400">
          {t("rollbackConfirmDescription")}
        </p>
        <div className="flex justify-end gap-3">
          <Button
            variant="outline"
            size="sm"
            onClick={() => setRollbackTarget(null)}
          >
            {tc("cancel")}
          </Button>
          <Button size="sm" onClick={handleRollback} loading={rolling}>
            {t("rollback")}
          </Button>
        </div>
      </Dialog>
    </div>
  );
}

export { DocumentHistory };
