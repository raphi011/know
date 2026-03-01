"use client";

import { forwardRef, type ComponentPropsWithoutRef } from "react";
import { cn } from "@/lib/utils";

type InputProps = ComponentPropsWithoutRef<"input"> & {
  error?: boolean;
};

const Input = forwardRef<HTMLInputElement, InputProps>(function Input(
  { className, error, type = "text", ...props },
  ref,
) {
  return (
    <input
      type={type}
      ref={ref}
      className={cn(
        "flex h-10 w-full rounded-xl bg-white px-3 py-2.5 text-sm",
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
});

export { Input };
export type { InputProps };
