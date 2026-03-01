"use client";

import { Fragment } from "react";
import {
  Menu,
  MenuButton,
  MenuItem,
  MenuItems,
  Transition,
} from "@headlessui/react";
import { cn } from "@/lib/utils";

type DropdownMenuProps = {
  trigger: React.ReactNode;
  children: React.ReactNode;
  align?: "left" | "right";
  className?: string;
};

function DropdownMenu({
  trigger,
  children,
  align = "right",
  className,
}: DropdownMenuProps) {
  return (
    <Menu as="div" className="relative text-left">
      <MenuButton as={Fragment}>{trigger}</MenuButton>
      <Transition
        as={Fragment}
        enter="transition ease-out duration-150"
        enterFrom="opacity-0 scale-95"
        enterTo="opacity-100 scale-100"
        leave="transition ease-in duration-100"
        leaveFrom="opacity-100 scale-100"
        leaveTo="opacity-0 scale-95"
      >
        <MenuItems
          className={cn(
            "absolute z-50 mt-2 min-w-[180px] rounded-xl bg-white p-1 shadow-md",
            "ring-1 ring-slate-200",
            "focus:outline-none",
            "dark:bg-slate-900 dark:ring-slate-800",
            align === "right" ? "right-0" : "left-0",
            className,
          )}
        >
          {children}
        </MenuItems>
      </Transition>
    </Menu>
  );
}

type DropdownMenuItemProps = {
  children: React.ReactNode;
  onClick?: () => void;
  destructive?: boolean;
  icon?: React.ReactNode;
  disabled?: boolean;
};

function DropdownMenuItem({
  children,
  onClick,
  destructive,
  icon,
  disabled,
}: DropdownMenuItemProps) {
  return (
    <MenuItem disabled={disabled}>
      {({ focus }) => (
        <button
          onClick={onClick}
          disabled={disabled}
          className={cn(
            "flex w-full items-center gap-2 rounded-lg px-3 py-2 text-sm",
            "transition-colors duration-100",
            destructive
              ? "text-red-600 dark:text-red-400"
              : "text-slate-700 dark:text-slate-300",
            focus &&
              (destructive
                ? "bg-red-50 dark:bg-red-950"
                : "bg-slate-100 dark:bg-slate-800"),
            disabled && "cursor-not-allowed opacity-50",
          )}
        >
          {icon && <span className="shrink-0 [&_svg]:size-4">{icon}</span>}
          {children}
        </button>
      )}
    </MenuItem>
  );
}

function DropdownMenuSeparator() {
  return <div className="my-1 h-px bg-slate-200 dark:bg-slate-800" />;
}

export { DropdownMenu, DropdownMenuItem, DropdownMenuSeparator };
export type { DropdownMenuProps, DropdownMenuItemProps };
