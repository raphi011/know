"use client";

import { useEffect, useLayoutEffect, useRef } from "react";
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

  // Clamp position to viewport bounds before paint via direct DOM mutation
  useLayoutEffect(() => {
    if (!open) return;
    const menu = menuRef.current;
    if (!menu) return;

    const rect = menu.getBoundingClientRect();
    const padding = 8;
    const x = Math.max(padding, Math.min(position.x, window.innerWidth - rect.width - padding));
    const y = Math.max(padding, Math.min(position.y, window.innerHeight - rect.height - padding));
    menu.style.left = `${x}px`;
    menu.style.top = `${y}px`;
  }, [open, position]);

  // Focus first menu item on open
  useEffect(() => {
    if (!open) return;
    const menu = menuRef.current;
    if (!menu) return;

    requestAnimationFrame(() => {
      const firstItem = menu.querySelector<HTMLElement>('[role="menuitem"]:not([disabled])');
      firstItem?.focus();
    });
  }, [open]);

  useEffect(() => {
    if (!open) return;

    // Defer listener registration to the next frame so the mousedown event
    // that opened this menu doesn't immediately trigger the close handler.
    let rafId: number;

    const handleClickOutside = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        onClose();
      }
    };

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        onClose();
        return;
      }

      if (e.key === "ArrowDown" || e.key === "ArrowUp") {
        e.preventDefault();
        const menu = menuRef.current;
        if (!menu) return;

        const items = Array.from(
          menu.querySelectorAll<HTMLElement>('[role="menuitem"]:not([disabled])'),
        );
        if (items.length === 0) return;

        const current = document.activeElement as HTMLElement;
        const currentIndex = items.indexOf(current);
        let nextIndex: number;

        if (e.key === "ArrowDown") {
          nextIndex = currentIndex < items.length - 1 ? currentIndex + 1 : 0;
        } else {
          nextIndex = currentIndex > 0 ? currentIndex - 1 : items.length - 1;
        }

        items[nextIndex]?.focus();
      }
    };

    const handleScroll = () => {
      onClose();
    };

    rafId = requestAnimationFrame(() => {
      document.addEventListener("mousedown", handleClickOutside);
      document.addEventListener("keydown", handleKeyDown);
      document.addEventListener("scroll", handleScroll, true);
    });

    return () => {
      cancelAnimationFrame(rafId);
      document.removeEventListener("mousedown", handleClickOutside);
      document.removeEventListener("keydown", handleKeyDown);
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
      tabIndex={-1}
      disabled={disabled}
      onClick={(e) => {
        e.stopPropagation();
        onClick?.();
      }}
      className={cn(
        "flex w-full items-center gap-2 rounded-lg px-3 py-2 text-sm",
        "transition-colors duration-100",
        "focus:outline-none",
        destructive
          ? "text-red-600 dark:text-red-400"
          : "text-slate-700 dark:text-slate-300",
        destructive
          ? "hover:bg-red-50 focus:bg-red-50 dark:hover:bg-red-950 dark:focus:bg-red-950"
          : "hover:bg-slate-100 focus:bg-slate-100 dark:hover:bg-slate-800 dark:focus:bg-slate-800",
        disabled && "cursor-not-allowed opacity-50",
      )}
    >
      {icon && <span className="shrink-0 [&_svg]:size-4">{icon}</span>}
      {children}
    </button>
  );
}

function ContextMenuSeparator() {
  return <div role="separator" className="my-1 h-px bg-slate-200 dark:bg-slate-800" />;
}

export { ContextMenu, ContextMenuItem, ContextMenuSeparator };
export type { Position, ContextMenuProps, ContextMenuItemProps };
