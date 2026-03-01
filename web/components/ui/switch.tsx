"use client";

import {
  Switch as HeadlessSwitch,
  Field,
  Label,
  type SwitchProps as HeadlessSwitchProps,
} from "@headlessui/react";
import { cn } from "@/lib/utils";

type SwitchProps = Omit<HeadlessSwitchProps, "as"> & {
  label?: string;
};

function Switch({ className, label, disabled, ...props }: SwitchProps) {
  const toggle = (
    <HeadlessSwitch
      disabled={disabled}
      className={cn(
        "group relative inline-flex h-6 w-11 shrink-0 items-center rounded-full",
        "bg-slate-200 transition-colors duration-150 ease-out",
        "data-[checked]:bg-primary-500 dark:data-[checked]:bg-primary-400",
        "focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 focus-visible:ring-offset-2",
        "disabled:cursor-not-allowed disabled:opacity-50",
        "dark:bg-slate-700 dark:focus-visible:ring-offset-slate-900",
        className,
      )}
      {...props}
    >
      <span
        aria-hidden="true"
        className={cn(
          "pointer-events-none inline-block size-4 translate-x-1 rounded-full bg-white shadow-sm",
          "ring-0 transition-transform duration-150 ease-out",
          "group-data-[checked]:translate-x-6",
        )}
      />
    </HeadlessSwitch>
  );

  if (!label) return toggle;

  return (
    <Field className="flex items-center gap-3">
      {toggle}
      <Label
        className={cn(
          "text-sm text-slate-700 dark:text-slate-300",
          disabled && "cursor-not-allowed opacity-50",
        )}
      >
        {label}
      </Label>
    </Field>
  );
}

export { Switch };
export type { SwitchProps };
