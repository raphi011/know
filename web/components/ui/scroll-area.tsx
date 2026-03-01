import { cn } from "@/lib/utils";

type ScrollAreaProps = {
  children: React.ReactNode;
  className?: string;
  orientation?: "vertical" | "horizontal";
};

function ScrollArea({
  children,
  className,
  orientation = "vertical",
}: ScrollAreaProps) {
  return (
    <div
      tabIndex={0}
      className={cn(
        "relative",
        orientation === "vertical"
          ? "overflow-y-auto overflow-x-hidden"
          : "overflow-x-auto overflow-y-hidden",
        // Styled scrollbar
        "[&::-webkit-scrollbar]:w-1.5",
        "[&::-webkit-scrollbar-track]:bg-transparent",
        "[&::-webkit-scrollbar-thumb]:rounded-full",
        "[&::-webkit-scrollbar-thumb]:bg-slate-300",
        "dark:[&::-webkit-scrollbar-thumb]:bg-slate-700",
        className,
      )}
    >
      {children}
    </div>
  );
}

export { ScrollArea };
export type { ScrollAreaProps };
