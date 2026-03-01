"use client";

import { useTranslations } from "next-intl";
import { ResponsiveDialog } from "@/components/responsive-dialog";
import { Button } from "@/components/ui/button";

type ConfirmDialogProps = {
  open: boolean;
  onClose: () => void;
  onConfirm: () => void;
  title: string;
  description?: string;
  error?: string;
  confirmLabel?: string;
  cancelLabel?: string;
  loading?: boolean;
};

function ConfirmDialog({
  open,
  onClose,
  onConfirm,
  title,
  description,
  error,
  confirmLabel,
  cancelLabel,
  loading,
}: ConfirmDialogProps) {
  const t = useTranslations("common");

  return (
    <ResponsiveDialog open={open} onClose={onClose} title={title}>
      {description && (
        <p className="mb-6 text-sm text-slate-500 dark:text-slate-400">
          {description}
        </p>
      )}
      {error && (
        <p className="mb-6 text-sm font-medium text-red-600 dark:text-red-400">
          {error}
        </p>
      )}
      <div className="flex gap-3">
        <Button
          variant="outline"
          className="flex-1"
          onClick={onClose}
          disabled={loading}
        >
          {cancelLabel ?? t("cancel")}
        </Button>
        <Button
          variant="destructive"
          className="flex-1"
          onClick={onConfirm}
          loading={loading}
        >
          {confirmLabel ?? t("confirm")}
        </Button>
      </div>
    </ResponsiveDialog>
  );
}

export { ConfirmDialog };
export type { ConfirmDialogProps };
