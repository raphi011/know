"use client";

import Link from "next/link";
import {
  Cog6ToothIcon,
  MagnifyingGlassIcon,
} from "@heroicons/react/24/outline";
import { useTranslations } from "next-intl";
import { Avatar } from "@/components/ui/avatar";
import { Separator } from "@/components/ui/separator";
import { routes } from "@/app/lib/routes";
import { cn } from "@/lib/utils";

export type NavItem = {
  icon: React.ReactNode;
  label: string;
  href: string;
  badge?: number;
};

export type ProfileInfo = {
  name: string;
  avatarSrc?: string | null;
  href: string;
};

type SidebarNavProps = {
  appName?: string;
  navSections: NavItem[];
  sidebarContent?: React.ReactNode;
  profile?: ProfileInfo;
  activeHref: string;
  onNavigate?: (href: string) => void;
  onSearchClick?: () => void;
  className?: string;
};

function SidebarNav({
  appName = "App",
  navSections,
  sidebarContent,
  profile,
  activeHref,
  onNavigate,
  onSearchClick,
  className,
}: SidebarNavProps) {
  const t = useTranslations("nav");
  const tSearch = useTranslations("search");

  return (
    <nav
      className={cn(
        "flex h-full w-60 flex-col",
        "bg-white ring-1 ring-slate-200",
        "dark:bg-slate-900 dark:ring-slate-800",
        className,
      )}
    >
      {/* Logo */}
      <Link
        href="/"
        className="flex items-center gap-4 px-6 pt-4 pb-4 transition-opacity hover:opacity-80"
      >
        <span className="text-base font-bold text-slate-900 dark:text-white">
          {appName}
        </span>
      </Link>

      {/* Search button */}
      {onSearchClick && (
        <div className="px-3 pb-2">
          <button
            onClick={onSearchClick}
            className={cn(
              "flex w-full items-center gap-3 rounded-xl px-3 py-2.5 text-sm font-medium",
              "text-slate-500 transition-colors duration-150",
              "bg-slate-50 hover:bg-slate-100",
              "dark:bg-slate-800/50 dark:text-slate-400 dark:hover:bg-slate-800",
              "[&_svg]:size-5 [&_svg]:shrink-0",
            )}
          >
            <MagnifyingGlassIcon />
            <span className="flex-1 text-left">{tSearch("hint")}</span>
            <kbd className="rounded-md bg-slate-200/70 px-1.5 py-0.5 text-[10px] font-semibold text-slate-400 dark:bg-slate-700 dark:text-slate-500">
              ⌘K
            </kbd>
          </button>
        </div>
      )}

      {/* Nav items */}
      <div className="flex-1 overflow-y-auto px-3">
        {sidebarContent ?? (
          <div className="space-y-1">
            {navSections.map((item) => (
              <NavButton
                key={item.href}
                item={item}
                active={activeHref === item.href}
                onNavigate={onNavigate}
              />
            ))}
          </div>
        )}
      </div>

      {/* Bottom: Settings + Profile */}
      <div className="px-3">
        <Separator />

        <div className="space-y-0.5 py-2">
          <NavButton
            item={{
              icon: <Cog6ToothIcon />,
              label: t("settings"),
              href: routes.settings,
            }}
            active={activeHref === routes.settings}
            onNavigate={onNavigate}
          />
        </div>

        {profile && (
          <div className="pb-3">
            <ProfileButton
              profile={profile}
              active={activeHref === profile.href}
              onNavigate={onNavigate}
            />
          </div>
        )}
      </div>
    </nav>
  );
}

function NavButton({
  item,
  active,
  onNavigate,
}: {
  item: NavItem;
  active: boolean;
  onNavigate?: (href: string) => void;
}) {
  return (
    <button
      onClick={() => onNavigate?.(item.href)}
      className={cn(
        "flex w-full items-center gap-3 rounded-xl px-3 py-2.5 text-sm font-medium",
        "transition-colors duration-150",
        active
          ? "bg-primary-50 text-primary-700 dark:bg-primary-950 dark:text-primary-400"
          : "text-slate-600 hover:bg-slate-50 dark:text-slate-400 dark:hover:bg-slate-800",
        "[&_svg]:size-5 [&_svg]:shrink-0",
      )}
      aria-current={active ? "page" : undefined}
    >
      {item.icon}
      <span className="flex-1 text-left">{item.label}</span>
      {item.badge != null && item.badge > 0 && (
        <span className="flex size-5 items-center justify-center rounded-full bg-slate-200 text-[10px] font-bold text-slate-700 dark:bg-slate-700 dark:text-slate-200">
          {item.badge > 9 ? "9+" : item.badge}
        </span>
      )}
    </button>
  );
}

function ProfileButton({
  profile,
  active,
  onNavigate,
}: {
  profile: ProfileInfo;
  active: boolean;
  onNavigate?: (href: string) => void;
}) {
  return (
    <button
      onClick={() => onNavigate?.(profile.href)}
      className={cn(
        "flex w-full items-center gap-3 rounded-xl px-3 py-2.5 text-sm font-medium",
        "transition-colors duration-150",
        active
          ? "bg-primary-50 text-primary-700 dark:bg-primary-950 dark:text-primary-400"
          : "text-slate-600 hover:bg-slate-50 dark:text-slate-400 dark:hover:bg-slate-800",
      )}
      aria-current={active ? "page" : undefined}
    >
      <Avatar name={profile.name} src={profile.avatarSrc} size="sm" />
      <span>{profile.name}</span>
    </button>
  );
}

export { SidebarNav, NavButton, ProfileButton };
export type { SidebarNavProps };
