"use client";

import { forwardRef, type ComponentPropsWithoutRef } from "react";
import { ChevronDownIcon } from "@heroicons/react/20/solid";
import { cn } from "@/lib/utils";

type SelectProps = ComponentPropsWithoutRef<"select"> & {
  error?: boolean;
};

const Select = forwardRef<HTMLSelectElement, SelectProps>(function Select(
  { className, error, children, ...props },
  ref,
) {
  return (
    <div className="relative">
      <select
        ref={ref}
        className={cn(
          "flex h-10 w-full appearance-none rounded-xl bg-white py-2.5 pl-3 pr-8 text-sm",
          "text-slate-900",
          "ring-1 ring-slate-300 transition-colors duration-150",
          "focus:outline-none focus:ring-2 focus:ring-primary-500",
          "disabled:cursor-not-allowed disabled:opacity-50",
          "dark:bg-slate-900 dark:text-white dark:ring-slate-700",
          error && "ring-red-500 focus:ring-red-500",
          className,
        )}
        {...props}
      >
        {children}
      </select>
      <ChevronDownIcon
        className="pointer-events-none absolute right-2.5 top-1/2 size-4 -translate-y-1/2 text-slate-500 dark:text-slate-400"
        aria-hidden="true"
      />
    </div>
  );
});

export { Select };
export type { SelectProps };
