"use client";

import { cn } from "@/lib/utils";

type FloatingFabProps = {
  icon: React.ReactNode;
  label: string;
  onClick: () => void;
  disabled?: boolean;
  variant?: "default" | "active";
};

function FloatingFab({
  icon,
  label,
  onClick,
  disabled,
  variant,
}: FloatingFabProps) {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      className={cn(
        "fixed right-4 z-40",
        "bottom-[calc(1rem+env(safe-area-inset-bottom))]",
        "flex size-14 items-center justify-center rounded-full",
        "shadow-lg",
        "transition-all duration-150",
        "disabled:opacity-50 disabled:shadow-sm",
        "[&_svg]:size-6",
        variant === "active"
          ? "bg-accent-400 text-white active:bg-accent-500"
          : "bg-primary-500 text-white active:bg-primary-700 active:shadow-md",
      )}
      aria-label={label}
    >
      {icon}
    </button>
  );
}

export { FloatingFab };
export type { FloatingFabProps };
