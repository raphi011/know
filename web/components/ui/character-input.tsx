"use client";

import { useRef, useCallback } from "react";
import { cn } from "@/lib/utils";

type CharacterInputProps = {
  length?: number;
  value: string;
  onChange: (value: string) => void;
  error?: boolean;
  className?: string;
};

function CharacterInput({
  length = 6,
  value,
  onChange,
  error,
  className,
}: CharacterInputProps) {
  const inputsRef = useRef<(HTMLInputElement | null)[]>([]);

  const chars = value.split("").concat(Array(length).fill("")).slice(0, length);

  const focusInput = useCallback(
    (index: number) => {
      const clamped = Math.max(0, Math.min(index, length - 1));
      inputsRef.current[clamped]?.focus();
    },
    [length],
  );

  function handleChange(index: number, char: string) {
    const sanitized = char.replace(/[^a-zA-Z0-9]/g, "").toUpperCase();
    if (!sanitized) return;

    const arr = chars.slice();
    arr[index] = sanitized[0] ?? "";
    onChange(arr.join(""));

    if (index < length - 1) {
      focusInput(index + 1);
    }
  }

  function handleKeyDown(index: number, e: React.KeyboardEvent) {
    if (e.key === "Backspace") {
      e.preventDefault();
      const arr = chars.slice();
      if (arr[index]) {
        arr[index] = "";
        onChange(arr.join(""));
      } else if (index > 0) {
        arr[index - 1] = "";
        onChange(arr.join(""));
        focusInput(index - 1);
      }
    } else if (e.key === "ArrowLeft") {
      focusInput(index - 1);
    } else if (e.key === "ArrowRight") {
      focusInput(index + 1);
    }
  }

  function handlePaste(e: React.ClipboardEvent) {
    e.preventDefault();
    const pasted = e.clipboardData
      .getData("text")
      .replace(/[^a-zA-Z0-9]/g, "")
      .toUpperCase()
      .slice(0, length);
    onChange(pasted);
    focusInput(pasted.length - 1);
  }

  return (
    <div className={cn("flex justify-center gap-2", className)}>
      {Array.from({ length }, (_, i) => (
        <input
          key={i}
          ref={(el) => {
            inputsRef.current[i] = el;
          }}
          type="text"
          inputMode="text"
          maxLength={1}
          value={chars[i] || ""}
          onChange={(e) => handleChange(i, e.target.value)}
          onKeyDown={(e) => handleKeyDown(i, e)}
          onPaste={handlePaste}
          className={cn(
            "size-12 rounded-xl bg-white text-center text-lg font-bold",
            "ring-1 ring-slate-300 transition-colors duration-150",
            "focus:outline-none focus:ring-2 focus:ring-primary-500",
            "dark:bg-slate-900 dark:text-white dark:ring-slate-700",
            error && "ring-red-500 focus:ring-red-500",
          )}
          aria-label={`Character ${i + 1}`}
        />
      ))}
    </div>
  );
}

export { CharacterInput };
export type { CharacterInputProps };
