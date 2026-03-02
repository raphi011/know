"use client";

import { useEffect, useRef } from "react";
import { createPortal } from "react-dom";
import { cn } from "@/lib/utils";

type Position = { x: number; y: number };

type ContextMenuProps = {
  open: boolean;
  position: Position;
  onClose: () => void;
  children: React.ReactNode;
};

function ContextMenu({ open, position, onClose, children }: ContextMenuProps) {
  const menuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;

    // Use rAF delay to avoid the right-click event itself triggering close
    let rafId: number;

    const handleClickOutside = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        onClose();
      }
    };

    const handleEscape = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        onClose();
      }
    };

    const handleScroll = () => {
      onClose();
    };

    rafId = requestAnimationFrame(() => {
      document.addEventListener("mousedown", handleClickOutside);
      document.addEventListener("keydown", handleEscape);
      document.addEventListener("scroll", handleScroll, true);
    });

    return () => {
      cancelAnimationFrame(rafId);
      document.removeEventListener("mousedown", handleClickOutside);
      document.removeEventListener("keydown", handleEscape);
      document.removeEventListener("scroll", handleScroll, true);
    };
  }, [open, onClose]);

  if (!open) return null;

  return createPortal(
    <div
      ref={menuRef}
      role="menu"
      className={cn(
        "fixed z-50 min-w-[180px] rounded-xl bg-white p-1 shadow-md",
        "ring-1 ring-slate-200",
        "focus:outline-none",
        "dark:bg-slate-900 dark:ring-slate-800",
      )}
      style={{ top: position.y, left: position.x }}
    >
      {children}
    </div>,
    document.body,
  );
}

type ContextMenuItemProps = {
  children: React.ReactNode;
  onClick?: () => void;
  destructive?: boolean;
  icon?: React.ReactNode;
  disabled?: boolean;
};

function ContextMenuItem({
  children,
  onClick,
  destructive,
  icon,
  disabled,
}: ContextMenuItemProps) {
  return (
    <button
      role="menuitem"
      disabled={disabled}
      onClick={(e) => {
        e.stopPropagation();
        onClick?.();
      }}
      className={cn(
        "flex w-full items-center gap-2 rounded-lg px-3 py-2 text-sm",
        "transition-colors duration-100",
        destructive
          ? "text-red-600 dark:text-red-400"
          : "text-slate-700 dark:text-slate-300",
        destructive
          ? "hover:bg-red-50 dark:hover:bg-red-950"
          : "hover:bg-slate-100 dark:hover:bg-slate-800",
        disabled && "cursor-not-allowed opacity-50",
      )}
    >
      {icon && <span className="shrink-0 [&_svg]:size-4">{icon}</span>}
      {children}
    </button>
  );
}

function ContextMenuSeparator() {
  return <div className="my-1 h-px bg-slate-200 dark:bg-slate-800" />;
}

export { ContextMenu, ContextMenuItem, ContextMenuSeparator };
export type { Position, ContextMenuProps, ContextMenuItemProps };
