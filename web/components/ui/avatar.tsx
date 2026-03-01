import { cn } from "@/lib/utils";

const sizeClasses = {
  sm: "size-8 text-xs",
  md: "size-10 text-sm",
  lg: "size-14 text-lg",
  xl: "size-20 text-2xl",
} as const;

const statusColors = {
  available: "ring-primary-500",
  active: "ring-orange-500",
  inactive: "ring-slate-400",
} as const;

export type AvatarSize = keyof typeof sizeClasses;
export type AvatarStatus = keyof typeof statusColors;

type AvatarProps = {
  src?: string | null;
  name: string;
  size?: AvatarSize;
  status?: AvatarStatus;
  className?: string;
};

function getInitials(name: string): string {
  return name
    .split(" ")
    .map((part) => part[0])
    .filter(Boolean)
    .slice(0, 2)
    .join("")
    .toUpperCase();
}

function Avatar({ src, name, size = "md", status, className }: AvatarProps) {
  const initials = getInitials(name);

  return (
    <span
      className={cn(
        "relative inline-flex shrink-0 items-center justify-center rounded-full",
        "bg-slate-200 font-semibold text-slate-600",
        "dark:bg-slate-700 dark:text-slate-300",
        sizeClasses[size],
        status && `ring-2 ${statusColors[status]}`,
        className,
      )}
    >
      {src ? (
        // eslint-disable-next-line @next/next/no-img-element -- avatars load from arbitrary external URLs
        <img
          src={src}
          alt={name}
          className="size-full rounded-full object-cover"
        />
      ) : (
        <span aria-hidden="true">{initials}</span>
      )}
      <span className="sr-only">{name}</span>
    </span>
  );
}

export { Avatar };
export type { AvatarProps };
