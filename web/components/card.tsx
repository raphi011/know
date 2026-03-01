import { cn } from "@/lib/utils";

type CardProps = {
  children: React.ReactNode;
  className?: string;
  interactive?: boolean;
  highlighted?: boolean;
};

function Card({ children, className, interactive, highlighted }: CardProps) {
  return (
    <div
      className={cn(
        "rounded-2xl bg-white p-4",
        "ring-1 ring-slate-200",
        "dark:bg-slate-900 dark:ring-slate-800",
        interactive && "transition-shadow duration-150 hover:shadow-sm",
        highlighted && "ring-primary-500 dark:ring-primary-500",
        className,
      )}
    >
      {children}
    </div>
  );
}

function CardHeader({
  children,
  className,
}: {
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("flex items-center justify-between", className)}>
      {children}
    </div>
  );
}

function CardTitle({
  children,
  className,
}: {
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <h3
      className={cn(
        "text-base font-semibold text-slate-900 dark:text-white",
        className,
      )}
    >
      {children}
    </h3>
  );
}

function CardAction({
  children,
  className,
}: {
  children: React.ReactNode;
  className?: string;
}) {
  return <div className={cn("ml-auto", className)}>{children}</div>;
}

function CardContent({
  children,
  className,
}: {
  children: React.ReactNode;
  className?: string;
}) {
  return <div className={cn("mt-3", className)}>{children}</div>;
}

function CardFooter({
  children,
  className,
}: {
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <div
      className={cn(
        "mt-4 flex items-center gap-2 border-t border-slate-100 pt-4 dark:border-slate-800",
        className,
      )}
    >
      {children}
    </div>
  );
}

export { Card, CardHeader, CardTitle, CardAction, CardContent, CardFooter };
export type { CardProps };
