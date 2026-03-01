import type { ComponentPropsWithoutRef } from "react";
import { cn } from "@/lib/utils";

const variants = {
  success:
    "bg-primary-100 text-primary-700 dark:bg-primary-950 dark:text-primary-400",
  error: "bg-red-100 text-red-700 dark:bg-red-950 dark:text-red-400",
  warning:
    "bg-accent-100 text-accent-600 dark:bg-accent-500/10 dark:text-accent-400",
  highlight: "bg-accent-500 text-white",
  info: "bg-sky-100 text-sky-700 dark:bg-sky-950 dark:text-sky-400",
  subtle:
    "bg-primary-50 text-primary-600 ring-1 ring-inset ring-primary-200 dark:bg-primary-950/30 dark:text-primary-400 dark:ring-primary-800",
} as const;

const sizes = {
  sm: "px-1.5 py-0.5 text-[10px]",
  default: "px-2 py-0.5 text-xs",
} as const;

export type BadgeVariant = keyof typeof variants;

type BadgeProps = ComponentPropsWithoutRef<"span"> & {
  variant?: BadgeVariant;
  size?: keyof typeof sizes;
};

function Badge({
  className,
  variant = "info",
  size = "default",
  ...props
}: BadgeProps) {
  return (
    <span
      className={cn(
        "inline-flex items-center rounded-full font-medium",
        variants[variant],
        sizes[size],
        className,
      )}
      {...props}
    />
  );
}

export { Badge };
export type { BadgeProps };
