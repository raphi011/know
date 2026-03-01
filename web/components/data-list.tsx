import { Skeleton } from "@/components/ui/skeleton";
import { Separator } from "@/components/ui/separator";
import { EmptyState } from "@/components/empty-state";
import { cn } from "@/lib/utils";

type DataListProps<T> = {
  items: T[];
  loading?: boolean;
  loadingCount?: number;
  empty?: {
    icon?: React.ReactNode;
    title: string;
    description?: string;
    action?: { label: string; onClick: () => void };
  };
  renderItem: (item: T, index: number) => React.ReactNode;
  keyExtractor: (item: T, index: number) => string | number;
  separator?: boolean;
  className?: string;
};

function DataList<T>({
  items,
  loading,
  loadingCount = 3,
  empty,
  renderItem,
  keyExtractor,
  separator = true,
  className,
}: DataListProps<T>) {
  if (loading) {
    return (
      <div className={cn("space-y-3", className)}>
        {Array.from({ length: loadingCount }, (_, i) => (
          <Skeleton key={i} className="h-16 w-full" />
        ))}
      </div>
    );
  }

  if (items.length === 0 && empty) {
    return (
      <EmptyState
        icon={empty.icon}
        title={empty.title}
        description={empty.description}
        action={empty.action}
        className={className}
      />
    );
  }

  return (
    <div className={className}>
      {items.map((item, index) => (
        <div key={keyExtractor(item, index)}>
          {separator && index > 0 && <Separator />}
          {renderItem(item, index)}
        </div>
      ))}
    </div>
  );
}

export { DataList };
export type { DataListProps };
