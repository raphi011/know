"use client";

import { forwardRef, type ComponentPropsWithoutRef } from "react";
import { cn } from "@/lib/utils";

type TextareaProps = ComponentPropsWithoutRef<"textarea"> & {
  error?: boolean;
};

const Textarea = forwardRef<HTMLTextAreaElement, TextareaProps>(
  function Textarea({ className, error, ...props }, ref) {
    return (
      <textarea
        ref={ref}
        className={cn(
          "flex min-h-[80px] w-full rounded-xl bg-white px-3 py-2.5 text-sm",
          "text-slate-900 placeholder:text-slate-400",
          "ring-1 ring-slate-300 transition-colors duration-150",
          "focus:outline-none focus:ring-2 focus:ring-primary-500",
          "disabled:cursor-not-allowed disabled:opacity-50",
          "dark:bg-slate-900 dark:text-white dark:ring-slate-700",
          error && "ring-red-500 focus:ring-red-500",
          className,
        )}
        {...props}
      />
    );
  },
);

export { Textarea };
export type { TextareaProps };
