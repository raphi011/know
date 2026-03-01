"use client";

import { forwardRef, type ComponentPropsWithoutRef } from "react";
import { cn } from "@/lib/utils";

const base = [
  "inline-flex items-center justify-center gap-2",
  "font-semibold text-sm",
  "rounded-xl",
  "transition-colors duration-150 ease-out",
  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 focus-visible:ring-offset-2",
  "disabled:pointer-events-none disabled:opacity-50",
  "[&_svg]:pointer-events-none [&_svg]:shrink-0",
];

const variants = {
  primary: [
    "bg-primary-500 text-white shadow-sm",
    "hover:bg-primary-600",
    "active:bg-primary-700 active:shadow-none",
  ],
  outline: [
    "bg-transparent ring-1 ring-slate-300 dark:ring-slate-700",
    "text-slate-700 dark:text-slate-300",
    "hover:bg-slate-50 dark:hover:bg-slate-800",
  ],
  destructive: [
    "bg-red-600 text-white shadow-sm",
    "hover:bg-red-700",
    "active:bg-red-800 active:shadow-none",
  ],
  ghost: [
    "bg-transparent",
    "text-slate-600 dark:text-slate-400",
    "hover:bg-slate-100 dark:hover:bg-slate-800",
  ],
} as const;

const sizes = {
  sm: "h-8 px-3 text-xs [&_svg]:size-4",
  md: "h-10 px-4 py-2.5 [&_svg]:size-5",
  lg: "h-12 px-6 text-base [&_svg]:size-5",
} as const;

export type ButtonVariant = keyof typeof variants;
export type ButtonSize = keyof typeof sizes;

type ButtonProps = ComponentPropsWithoutRef<"button"> & {
  variant?: ButtonVariant;
  size?: ButtonSize;
  loading?: boolean;
  href?: string;
};

function Spinner() {
  return (
    <svg
      className="size-4 animate-spin"
      xmlns="http://www.w3.org/2000/svg"
      fill="none"
      viewBox="0 0 24 24"
      aria-hidden="true"
    >
      <circle
        className="opacity-25"
        cx="12"
        cy="12"
        r="10"
        stroke="currentColor"
        strokeWidth="4"
      />
      <path
        className="opacity-75"
        fill="currentColor"
        d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
      />
    </svg>
  );
}

const Button = forwardRef<HTMLButtonElement | HTMLAnchorElement, ButtonProps>(
  function Button(
    {
      className,
      variant = "primary",
      size = "md",
      loading = false,
      disabled,
      href,
      children,
      ...props
    },
    ref,
  ) {
    const classes = cn(base, variants[variant], sizes[size], className);

    if (href) {
      return (
        <a
          href={href}
          ref={ref as React.Ref<HTMLAnchorElement>}
          className={classes}
          aria-disabled={disabled || loading || undefined}
          {...(props as ComponentPropsWithoutRef<"a">)}
        >
          {loading && <Spinner />}
          {children}
        </a>
      );
    }

    return (
      <button
        ref={ref as React.Ref<HTMLButtonElement>}
        className={classes}
        disabled={disabled || loading}
        {...(props as ComponentPropsWithoutRef<"button">)}
      >
        {loading && <Spinner />}
        {children}
      </button>
    );
  },
);

export { Button };
export type { ButtonProps };
