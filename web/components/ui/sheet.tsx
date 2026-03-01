"use client";

import { Fragment } from "react";
import {
  Dialog as HeadlessDialog,
  DialogPanel,
  DialogTitle,
  Transition,
  TransitionChild,
} from "@headlessui/react";
import { XMarkIcon } from "@heroicons/react/20/solid";
import { cn } from "@/lib/utils";

type SheetProps = {
  open: boolean;
  onClose: () => void;
  title?: string;
  children: React.ReactNode;
  className?: string;
};

function Sheet({ open, onClose, title, children, className }: SheetProps) {
  return (
    <Transition show={open} as={Fragment}>
      <HeadlessDialog onClose={onClose} className="relative z-50">
        {/* Backdrop */}
        <TransitionChild
          as={Fragment}
          enter="ease-out duration-250"
          enterFrom="opacity-0"
          enterTo="opacity-100"
          leave="ease-in duration-150"
          leaveFrom="opacity-100"
          leaveTo="opacity-0"
        >
          <div className="fixed inset-0 bg-black/50" aria-hidden="true" />
        </TransitionChild>

        {/* Bottom sheet panel */}
        <div className="fixed inset-x-0 bottom-0 z-50">
          <TransitionChild
            as={Fragment}
            enter="ease-out duration-250"
            enterFrom="translate-y-full"
            enterTo="translate-y-0"
            leave="ease-in duration-150"
            leaveFrom="translate-y-0"
            leaveTo="translate-y-full"
          >
            <DialogPanel
              className={cn(
                "w-full rounded-t-3xl bg-white p-6 shadow-xl",
                "dark:bg-slate-900",
                className,
              )}
            >
              {/* Drag handle */}
              <div className="mb-4 flex justify-center">
                <div className="h-1 w-10 rounded-full bg-slate-300 dark:bg-slate-700" />
              </div>

              {title && (
                <div className="mb-4 flex items-center justify-between">
                  <DialogTitle className="text-lg font-semibold text-slate-900 dark:text-white">
                    {title}
                  </DialogTitle>
                  <button
                    onClick={onClose}
                    className="rounded-lg p-1 text-slate-500 hover:bg-slate-100 hover:text-slate-600 dark:hover:bg-slate-800 dark:hover:text-slate-300"
                  >
                    <XMarkIcon className="size-5" />
                    <span className="sr-only">Close</span>
                  </button>
                </div>
              )}
              {children}
            </DialogPanel>
          </TransitionChild>
        </div>
      </HeadlessDialog>
    </Transition>
  );
}

export { Sheet };
export type { SheetProps };
