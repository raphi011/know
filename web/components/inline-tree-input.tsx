"use client";

import { useEffect, useRef, useState } from "react";
import {
  FolderIcon,
  DocumentTextIcon,
} from "@heroicons/react/24/outline";
import { cn } from "@/lib/utils";

type InlineTreeInputProps = {
  type: "document" | "folder";
  depth: number;
  defaultValue?: string;
  siblingNames: string[];
  onConfirm: (name: string) => void;
  onCancel: () => void;
  placeholder?: string;
};

function InlineTreeInput({
  type,
  depth,
  defaultValue,
  siblingNames,
  onConfirm,
  onCancel,
  placeholder,
}: InlineTreeInputProps) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const input = inputRef.current;
    if (!input) return;

    input.focus();

    if (defaultValue) {
      // Select the name part before the extension
      const dotIndex = defaultValue.lastIndexOf(".");
      if (dotIndex > 0) {
        input.setSelectionRange(0, dotIndex);
      } else {
        input.select();
      }
    }
  }, [defaultValue]);

  function validate(value: string): string | null {
    const trimmed = value.trim();

    if (trimmed === "") {
      return "nameRequired";
    }

    if (trimmed.includes("/")) {
      return "nameInvalid";
    }

    // For documents, auto-append .md when checking for duplicates
    const nameToCheck =
      type === "document" && !trimmed.endsWith(".md")
        ? `${trimmed}.md`
        : trimmed;

    const isDuplicate = siblingNames.some(
      (s) => s.toLowerCase() === nameToCheck.toLowerCase(),
    );

    if (isDuplicate) {
      return "nameDuplicate";
    }

    return null;
  }

  function handleKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key === "Enter") {
      e.preventDefault();
      const value = e.currentTarget.value;
      const validationError = validate(value);

      if (validationError) {
        setError(validationError);
        return;
      }

      onConfirm(value.trim());
    } else if (e.key === "Escape") {
      e.preventDefault();
      onCancel();
    }
  }

  function handleChange() {
    // Clear error on input change
    if (error) {
      setError(null);
    }
  }

  const Icon = type === "folder" ? FolderIcon : DocumentTextIcon;

  return (
    <div
      className="flex items-center gap-2 px-2 py-1.5"
      style={{ paddingLeft: `${depth * 16 + 8}px` }}
    >
      <span className="size-3.5 shrink-0" />
      <Icon className="size-4 shrink-0 text-slate-400" />
      <input
        ref={inputRef}
        type="text"
        defaultValue={defaultValue}
        placeholder={placeholder}
        onKeyDown={handleKeyDown}
        onChange={handleChange}
        onBlur={onCancel}
        className={cn(
          "min-w-0 flex-1 rounded border bg-white px-1.5 py-0.5 text-sm outline-none dark:bg-slate-900",
          error
            ? "border-red-400 focus:ring-1 focus:ring-red-400"
            : "border-slate-300 focus:border-primary-400 focus:ring-1 focus:ring-primary-400 dark:border-slate-700",
        )}
      />
    </div>
  );
}

export { InlineTreeInput };
export type { InlineTreeInputProps };
