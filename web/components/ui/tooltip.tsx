"use client";

import { useState } from "react";
import { cn } from "@/lib/utils";

type TooltipProps = {
  content: React.ReactNode;
  children: React.ReactElement;
  className?: string;
};

function Tooltip({ content, children, className }: TooltipProps) {
  const [open, setOpen] = useState(false);

  return (
    <span
      className="relative inline-flex"
      onMouseEnter={() => setOpen(true)}
      onMouseLeave={() => setOpen(false)}
      onFocus={() => setOpen(true)}
      onBlur={() => setOpen(false)}
    >
      {children}
      {open && (
        <span
          role="tooltip"
          className={cn(
            "absolute bottom-full left-1/2 z-50 mb-2 -translate-x-1/2",
            "whitespace-nowrap rounded-lg px-2.5 py-1.5 text-xs font-medium",
            "bg-slate-900 text-white shadow-md",
            "dark:bg-slate-100 dark:text-slate-900",
            "animate-in fade-in-0 zoom-in-95",
            className,
          )}
        >
          {content}
        </span>
      )}
    </span>
  );
}

export { Tooltip };
export type { TooltipProps };
