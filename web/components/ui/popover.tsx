"use client";

import { Fragment } from "react";
import {
  Popover as HeadlessPopover,
  PopoverButton,
  PopoverPanel,
  Transition,
} from "@headlessui/react";
import { cn } from "@/lib/utils";

type PopoverProps = {
  trigger: React.ReactNode;
  children: React.ReactNode;
  className?: string;
};

function Popover({ trigger, children, className }: PopoverProps) {
  return (
    <HeadlessPopover className="relative">
      <PopoverButton as={Fragment}>{trigger}</PopoverButton>
      <Transition
        as={Fragment}
        enter="transition ease-out duration-150"
        enterFrom="opacity-0 translate-y-1"
        enterTo="opacity-100 translate-y-0"
        leave="transition ease-in duration-100"
        leaveFrom="opacity-100 translate-y-0"
        leaveTo="opacity-0 translate-y-1"
      >
        <PopoverPanel
          className={cn(
            "absolute left-1/2 z-50 mt-2 -translate-x-1/2",
            "rounded-xl bg-white p-4 shadow-md",
            "ring-1 ring-slate-200",
            "dark:bg-slate-900 dark:ring-slate-800",
            className,
          )}
        >
          {children}
        </PopoverPanel>
      </Transition>
    </HeadlessPopover>
  );
}

export { Popover };
export type { PopoverProps };
