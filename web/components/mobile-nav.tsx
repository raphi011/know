"use client";

import { Fragment, useEffect } from "react";
import { usePathname } from "next/navigation";
import {
  Dialog as HeadlessDialog,
  DialogPanel,
  Transition,
  TransitionChild,
} from "@headlessui/react";
import Link from "next/link";
import { XMarkIcon } from "@heroicons/react/20/solid";
import { Cog6ToothIcon } from "@heroicons/react/24/outline";
import { useTranslations } from "next-intl";
import { Separator } from "@/components/ui/separator";
import {
  NavButton,
  ProfileButton,
  type ProfileInfo,
  type NavItem,
} from "@/components/sidebar-nav";
import { cn } from "@/lib/utils";

type MobileNavProps = {
  open: boolean;
  onClose: () => void;
  appName?: string;
  navSections: NavItem[];
  sidebarContent?: React.ReactNode;
  profile?: ProfileInfo;
  activeHref: string;
  onNavigate?: (href: string) => void;
};

function MobileNav({
  open,
  onClose,
  appName = "App",
  navSections,
  sidebarContent,
  profile,
  activeHref,
  onNavigate,
}: MobileNavProps) {
  const tNav = useTranslations("nav");
  const pathname = usePathname();

  // Close mobile nav when route changes (e.g. clicking a Link in sidebarContent)
  useEffect(() => {
    if (open) onClose();
  }, [pathname]); // eslint-disable-line react-hooks/exhaustive-deps

  function handleNavigate(href: string) {
    onNavigate?.(href);
    onClose();
  }

  return (
    <Transition show={open} as={Fragment}>
      <HeadlessDialog
        onClose={onClose}
        className="relative z-50"
        aria-label={tNav("menuLabel")}
      >
        {/* Fullscreen panel */}
        <TransitionChild
          as={Fragment}
          enter="ease-out duration-250"
          enterFrom="opacity-0"
          enterTo="opacity-100"
          leave="ease-in duration-150"
          leaveFrom="opacity-100"
          leaveTo="opacity-0"
        >
          <DialogPanel className="fixed inset-0 flex flex-col bg-white dark:bg-slate-900">
            {/* Header */}
            <div className="flex items-center justify-between px-4 py-3">
              <Link
                href="/"
                onClick={onClose}
                className="text-base font-bold text-slate-900 transition-opacity hover:opacity-80 dark:text-white"
              >
                {appName}
              </Link>
              <button
                onClick={onClose}
                className={cn(
                  "ml-3 flex size-11 shrink-0 items-center justify-center rounded-xl",
                  "text-slate-500 hover:bg-slate-100 dark:hover:bg-slate-800",
                )}
                aria-label={tNav("closeMenu")}
              >
                <XMarkIcon className="size-5" />
              </button>
            </div>

            <Separator />

            {/* Nav items */}
            <div
              className="flex-1 overflow-y-auto px-3 py-3"
              onClick={(e) => {
                // Close nav when any link is clicked (covers same-page clicks
                // where pathname doesn't change and the useEffect won't fire)
                if ((e.target as HTMLElement).closest("a")) onClose();
              }}
            >
              {sidebarContent ?? (
                <div className="space-y-1">
                  {navSections.map((item) => (
                    <NavButton
                      key={item.href}
                      item={item}
                      active={activeHref === item.href}
                      onNavigate={handleNavigate}
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
                    label: tNav("settings"),
                    href: "/settings",
                  }}
                  active={activeHref === "/settings"}
                  onNavigate={handleNavigate}
                />
              </div>

              {profile && (
                <div className="pb-3">
                  <ProfileButton
                    profile={profile}
                    active={activeHref === profile.href}
                    onNavigate={handleNavigate}
                  />
                </div>
              )}
            </div>
          </DialogPanel>
        </TransitionChild>
      </HeadlessDialog>
    </Transition>
  );
}

export { MobileNav };
export type { MobileNavProps };
