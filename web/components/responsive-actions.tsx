"use client";

import { ChevronDownIcon } from "@heroicons/react/20/solid";
import { Button } from "@/components/ui/button";
import { DropdownMenu, DropdownMenuItem } from "@/components/ui/dropdown-menu";
import { cn } from "@/lib/utils";

type Action = {
  label: string;
  onClick: () => void;
  variant?: "primary" | "ghost" | "destructive";
  disabled?: boolean;
};

type ResponsiveActionsProps = {
  actions: Action[];
  mobileLabel: string;
  className?: string;
};

const buttonVariant: Record<
  NonNullable<Action["variant"]>,
  "primary" | "ghost"
> = {
  primary: "primary",
  ghost: "ghost",
  destructive: "ghost",
};

function ResponsiveActions({
  actions,
  mobileLabel,
  className,
}: ResponsiveActionsProps) {
  if (actions.length === 0) return null;

  return (
    <div className={className}>
      {/* Desktop: inline buttons in caller-defined order */}
      <div className="hidden items-center gap-3 lg:flex">
        {actions.map((a) => (
          <Button
            key={a.label}
            size="sm"
            variant={buttonVariant[a.variant ?? "primary"] ?? "primary"}
            className={cn(
              a.variant === "destructive" &&
                "text-red-600 hover:text-red-700 dark:text-red-400 dark:hover:text-red-300",
              a.variant === "ghost" && "text-slate-500 dark:text-slate-400",
            )}
            onClick={a.onClick}
            disabled={a.disabled}
          >
            {a.label}
          </Button>
        ))}
      </div>

      {/* Mobile: labeled dropdown */}
      <div className="lg:hidden">
        <DropdownMenu
          trigger={
            <Button variant="outline" size="sm">
              {mobileLabel}
              <ChevronDownIcon className="size-4" />
            </Button>
          }
        >
          {actions.map((a) => (
            <DropdownMenuItem
              key={a.label}
              onClick={a.onClick}
              destructive={a.variant === "destructive"}
              disabled={a.disabled}
            >
              {a.label}
            </DropdownMenuItem>
          ))}
        </DropdownMenu>
      </div>
    </div>
  );
}

export { ResponsiveActions };
export type { ResponsiveActionsProps, Action as ResponsiveAction };
