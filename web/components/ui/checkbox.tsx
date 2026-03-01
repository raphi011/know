"use client";

import {
  Checkbox as HeadlessCheckbox,
  Field,
  Label,
  type CheckboxProps as HeadlessCheckboxProps,
} from "@headlessui/react";
import { CheckIcon } from "@heroicons/react/20/solid";
import { cn } from "@/lib/utils";

type CheckboxProps = Omit<HeadlessCheckboxProps, "as"> & {
  label?: string;
};

function Checkbox({ className, label, disabled, ...props }: CheckboxProps) {
  const box = (
    <HeadlessCheckbox
      disabled={disabled}
      className={cn(
        "group flex size-5 items-center justify-center rounded-md",
        "ring-1 ring-slate-300 transition-colors duration-150",
        "data-[checked]:bg-primary-500 data-[checked]:ring-primary-500",
        "focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 focus-visible:ring-offset-2",
        "disabled:cursor-not-allowed disabled:opacity-50",
        "dark:ring-slate-700 dark:data-[checked]:ring-primary-500",
        className,
      )}
      {...props}
    >
      <CheckIcon className="size-3.5 text-white opacity-0 group-data-[checked]:opacity-100" />
    </HeadlessCheckbox>
  );

  if (!label) return box;

  return (
    <Field className="flex items-center gap-2">
      {box}
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

export { Checkbox };
export type { CheckboxProps };
