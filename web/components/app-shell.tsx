"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { Bars3Icon, MagnifyingGlassIcon } from "@heroicons/react/24/outline";
import { cn } from "@/lib/utils";
import { MobileNav } from "@/components/mobile-nav";
import { FloatingFab } from "@/components/floating-fab";
import {
  SidebarNav,
  type ProfileInfo,
  type NavItem,
} from "@/components/sidebar-nav";

type AppShellProps = {
  children: React.ReactNode;
  appName?: string;
  navSections: NavItem[];
  sidebarContent?: React.ReactNode;
  profile?: ProfileInfo;
  activeHref: string;
  onNavigate?: (href: string) => void;
  onSearchClick?: () => void;
  fab?: {
    icon: React.ReactNode;
    label: string;
    onClick: () => void;
    disabled?: boolean;
    variant?: "default" | "active";
  };
  className?: string;
};

function AppShell({
  children,
  appName = "App",
  navSections,
  sidebarContent,
  profile,
  activeHref,
  onNavigate,
  onSearchClick,
  fab,
  className,
}: AppShellProps) {
  const [mobileNavOpen, setMobileNavOpen] = useState(false);
  const tNav = useTranslations("nav");

  return (
    <div
      className={cn("min-h-screen bg-slate-50 dark:bg-slate-950", className)}
    >
      {/* Desktop sidebar */}
      <div className="hidden lg:fixed lg:inset-y-0 lg:flex lg:w-60">
        <SidebarNav
          appName={appName}
          navSections={navSections}
          sidebarContent={sidebarContent}
          profile={profile}
          activeHref={activeHref}
          onNavigate={onNavigate}
          onSearchClick={onSearchClick}
        />
      </div>

      {/* Mobile top bar */}
      <header
        className={cn(
          "sticky top-0 z-40 flex items-center justify-between px-4 py-2",
          "bg-white/95 backdrop-blur-sm",
          "dark:bg-slate-950/95",
          "lg:hidden",
        )}
      >
        {/* Hamburger */}
        <button
          onClick={() => setMobileNavOpen(true)}
          className="flex size-11 items-center justify-center rounded-xl text-slate-700 dark:text-slate-300"
          aria-label={tNav("openMenu")}
        >
          <Bars3Icon className="size-6" />
        </button>

        {/* App title */}
        <span className="text-base font-bold text-slate-900 dark:text-white">
          {appName}
        </span>

        {/* Search button (mobile) */}
        {onSearchClick ? (
          <button
            onClick={onSearchClick}
            className="flex size-11 items-center justify-center rounded-xl text-slate-700 dark:text-slate-300"
            aria-label={tNav("search")}
          >
            <MagnifyingGlassIcon className="size-6" />
          </button>
        ) : (
          <div className="size-11" />
        )}
      </header>

      {/* Main content */}
      <main
        className={cn(
          "@container overflow-x-hidden px-4 pb-24 pt-2",
          "lg:ml-60 lg:px-6 lg:pb-6 lg:pt-6",
        )}
      >
        {children}
      </main>

      {/* Mobile nav overlay */}
      <MobileNav
        open={mobileNavOpen}
        onClose={() => setMobileNavOpen(false)}
        appName={appName}
        navSections={navSections}
        sidebarContent={sidebarContent}
        profile={profile}
        activeHref={activeHref}
        onNavigate={onNavigate}
      />

      {/* Floating FAB (mobile only) */}
      {fab && (
        <div className="lg:hidden">
          <FloatingFab {...fab} />
        </div>
      )}
    </div>
  );
}

export { AppShell };
export type { AppShellProps };
