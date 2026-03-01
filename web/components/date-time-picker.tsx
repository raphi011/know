"use client";

import { useState } from "react";
import { CalendarDaysIcon } from "@heroicons/react/20/solid";
import { Popover } from "@/components/ui/popover";
import { Calendar } from "@/components/ui/calendar";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";

type DateTimePickerProps = {
  id?: string;
  value?: Date;
  onChange?: (date: Date | undefined) => void;
  showTime?: boolean;
  placeholder?: string;
  className?: string;
};

function formatDate(date: Date, showTime: boolean): string {
  const dateStr = date.toLocaleDateString("en-US", {
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
  });
  if (!showTime) return dateStr;
  const timeStr = date.toLocaleTimeString("en-US", {
    hour: "2-digit",
    minute: "2-digit",
  });
  return `${dateStr} ${timeStr}`;
}

function DateTimePicker({
  id,
  value,
  onChange,
  showTime = false,
  placeholder = "Select date...",
  className,
}: DateTimePickerProps) {
  const [time, setTime] = useState(
    value
      ? `${String(value.getHours()).padStart(2, "0")}:${String(value.getMinutes()).padStart(2, "0")}`
      : "12:00",
  );

  function handleDateChange(date: Date | undefined) {
    if (!date) {
      onChange?.(undefined);
      return;
    }
    if (showTime) {
      const [h, m] = time.split(":").map(Number);
      date.setHours(h || 0, m || 0);
    }
    onChange?.(date);
  }

  function handleTimeChange(e: React.ChangeEvent<HTMLInputElement>) {
    setTime(e.target.value);
    if (value) {
      const [h, m] = e.target.value.split(":").map(Number);
      const next = new Date(value);
      next.setHours(h || 0, m || 0);
      onChange?.(next);
    }
  }

  return (
    <Popover
      trigger={
        <Button
          id={id}
          variant="outline"
          className={cn("justify-start", className)}
        >
          <CalendarDaysIcon className="size-4 text-slate-500" />
          <span className={cn(!value && "text-slate-500")}>
            {value ? formatDate(value, showTime) : placeholder}
          </span>
        </Button>
      }
    >
      <div className="space-y-3">
        <Calendar value={value} onChange={handleDateChange} />
        {showTime && (
          <div className="flex items-center gap-2 border-t border-slate-200 pt-3 dark:border-slate-700">
            <label className="text-xs font-medium text-slate-500">Time</label>
            <Input
              type="time"
              value={time}
              onChange={handleTimeChange}
              className="w-auto"
            />
          </div>
        )}
      </div>
    </Popover>
  );
}

export { DateTimePicker };
export type { DateTimePickerProps };
