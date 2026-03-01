import { ArrowUpIcon, ArrowDownIcon } from "@heroicons/react/20/solid";
import { cn } from "@/lib/utils";

type StatBlockProps = {
  label: string;
  value: string | number;
  trend?: "up" | "down" | "neutral";
  trendValue?: string;
  className?: string;
};

function StatBlock({
  label,
  value,
  trend,
  trendValue,
  className,
}: StatBlockProps) {
  return (
    <div className={cn("space-y-1", className)}>
      <p className="text-xs font-medium tracking-wide text-slate-500 dark:text-slate-300">
        {label}
      </p>
      <div className="flex items-baseline gap-2">
        <p className="text-2xl font-bold tracking-tight text-slate-900 dark:text-white">
          {value}
        </p>
        {trend && trend !== "neutral" && (
          <span
            className={cn(
              "inline-flex items-center gap-0.5 text-xs font-medium",
              trend === "up" && "text-primary-600 dark:text-primary-400",
              trend === "down" && "text-red-600 dark:text-red-400",
            )}
          >
            {trend === "up" ? (
              <ArrowUpIcon className="size-3.5" />
            ) : (
              <ArrowDownIcon className="size-3.5" />
            )}
            {trendValue}
          </span>
        )}
      </div>
    </div>
  );
}

export { StatBlock };
export type { StatBlockProps };
