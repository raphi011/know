"use client";

import { useEffect, useState } from "react";
import {
  CheckCircleIcon,
  ExclamationCircleIcon,
  InformationCircleIcon,
  XMarkIcon,
} from "@heroicons/react/20/solid";
import { cn } from "@/lib/utils";

type ToastVariant = "success" | "error" | "info";

type ToastProps = {
  variant?: ToastVariant;
  title: string;
  description?: string;
  open?: boolean;
  onClose?: () => void;
  action?: { label: string; onClick: () => void };
};

const icons: Record<ToastVariant, React.ReactNode> = {
  success: <CheckCircleIcon className="size-5 text-primary-500" />,
  error: <ExclamationCircleIcon className="size-5 text-red-600" />,
  info: <InformationCircleIcon className="size-5 text-sky-500" />,
};

function Toast({
  variant = "info",
  title,
  description,
  open = true,
  onClose,
  action,
}: ToastProps) {
  const [visible, setVisible] = useState(open);

  useEffect(() => {
    setVisible(open);
  }, [open]);

  if (!visible) return null;

  return (
    <div
      role="alert"
      className={cn(
        "flex w-full max-w-sm items-start gap-3 rounded-xl bg-white p-4 shadow-lg",
        "ring-1 ring-slate-200",
        "dark:bg-slate-900 dark:ring-slate-800",
        "animate-in slide-in-from-bottom-2 fade-in-0 duration-150",
      )}
    >
      <span className="shrink-0 pt-0.5">{icons[variant]}</span>
      <div className="flex-1">
        <p className="text-sm font-semibold text-slate-900 dark:text-white">
          {title}
        </p>
        {description && (
          <p className="mt-0.5 text-sm text-slate-500 dark:text-slate-400">
            {description}
          </p>
        )}
        {action && (
          <button
            onClick={action.onClick}
            className="mt-2 text-sm font-semibold text-primary-600 hover:text-primary-700 dark:text-primary-400"
          >
            {action.label}
          </button>
        )}
      </div>
      {onClose && (
        <button
          onClick={() => {
            setVisible(false);
            onClose();
          }}
          className="shrink-0 rounded-lg p-0.5 text-slate-500 hover:text-slate-600 dark:hover:text-slate-300"
        >
          <XMarkIcon className="size-4" />
          <span className="sr-only">Close</span>
        </button>
      )}
    </div>
  );
}

export { Toast };
export type { ToastProps, ToastVariant };
