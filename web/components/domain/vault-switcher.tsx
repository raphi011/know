"use client";

import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import {
  ServerStackIcon,
  CircleStackIcon,
  ChevronUpDownIcon,
} from "@heroicons/react/24/outline";
import {
  Listbox,
  ListboxButton,
  ListboxOption,
  ListboxOptions,
  Transition,
} from "@headlessui/react";
import { Fragment } from "react";
import { cn } from "@/lib/utils";
import {
  setActiveConnectionAction,
  setActiveVaultAction,
} from "@/app/lib/actions/connections";
import type { Vault, ServerConnection } from "@/app/lib/knowhow/types";

type VaultSwitcherProps = {
  connections: ServerConnection[];
  activeConnectionId: string | null;
  vaults: Vault[];
  activeVaultId: string | null;
};

function VaultSwitcher({
  connections,
  activeConnectionId,
  vaults,
  activeVaultId,
}: VaultSwitcherProps) {
  const router = useRouter();
  const t = useTranslations("vaultSwitcher");

  const activeConnection = connections.find(
    (c) => c.id === activeConnectionId,
  );
  const activeVault = vaults.find((v) => v.id === activeVaultId);

  async function handleConnectionChange(connectionId: string) {
    if (connectionId === activeConnectionId) return;
    await setActiveConnectionAction(connectionId);
    router.refresh();
  }

  async function handleVaultChange(vaultId: string) {
    if (vaultId === activeVaultId) return;
    await setActiveVaultAction(vaultId);
    router.refresh();
  }

  if (connections.length === 0) {
    return null;
  }

  return (
    <div className="space-y-1.5 px-0 pb-2">
      {/* Server selector — only show if multiple connections */}
      {connections.length > 1 && (
        <Listbox value={activeConnectionId ?? ""} onChange={handleConnectionChange}>
          <div className="relative">
            <ListboxButton
              className={cn(
                "flex w-full items-center gap-2 rounded-xl px-3 py-2 text-xs font-medium",
                "text-slate-500 transition-colors duration-150",
                "hover:bg-slate-50 dark:text-slate-400 dark:hover:bg-slate-800",
              )}
            >
              <ServerStackIcon className="size-4 shrink-0" />
              <span className="flex-1 truncate text-left">
                {activeConnection?.name ?? t("noServer")}
              </span>
              <ChevronUpDownIcon className="size-3.5 shrink-0 text-slate-400" />
            </ListboxButton>
            <Transition
              as={Fragment}
              leave="transition ease-in duration-100"
              leaveFrom="opacity-100"
              leaveTo="opacity-0"
            >
              <ListboxOptions
                className={cn(
                  "absolute z-50 mt-1 max-h-48 w-full overflow-auto rounded-xl py-1 shadow-lg",
                  "bg-white ring-1 ring-slate-200",
                  "dark:bg-slate-900 dark:ring-slate-700",
                )}
              >
                {connections.map((conn) => (
                  <ListboxOption
                    key={conn.id}
                    value={conn.id}
                    className={({ focus }) =>
                      cn(
                        "flex cursor-pointer items-center gap-2 px-3 py-1.5 text-xs",
                        focus
                          ? "bg-primary-50 text-primary-700 dark:bg-primary-950 dark:text-primary-400"
                          : "text-slate-700 dark:text-slate-300",
                      )
                    }
                  >
                    <ServerStackIcon className="size-3.5 shrink-0" />
                    <span className="truncate">{conn.name}</span>
                  </ListboxOption>
                ))}
              </ListboxOptions>
            </Transition>
          </div>
        </Listbox>
      )}

      {/* Vault selector */}
      {vaults.length > 1 && (
        <Listbox value={activeVaultId ?? ""} onChange={handleVaultChange}>
          <div className="relative">
            <ListboxButton
              className={cn(
                "flex w-full items-center gap-2 rounded-xl px-3 py-2 text-xs font-medium",
                "text-slate-600 transition-colors duration-150",
                "hover:bg-slate-50 dark:text-slate-300 dark:hover:bg-slate-800",
              )}
            >
              <CircleStackIcon className="size-4 shrink-0" />
              <span className="flex-1 truncate text-left">
                {activeVault?.name ?? t("noVault")}
              </span>
              <ChevronUpDownIcon className="size-3.5 shrink-0 text-slate-400" />
            </ListboxButton>
            <Transition
              as={Fragment}
              leave="transition ease-in duration-100"
              leaveFrom="opacity-100"
              leaveTo="opacity-0"
            >
              <ListboxOptions
                className={cn(
                  "absolute z-50 mt-1 max-h-48 w-full overflow-auto rounded-xl py-1 shadow-lg",
                  "bg-white ring-1 ring-slate-200",
                  "dark:bg-slate-900 dark:ring-slate-700",
                )}
              >
                {vaults.map((v) => (
                  <ListboxOption
                    key={v.id}
                    value={v.id}
                    className={({ focus }) =>
                      cn(
                        "flex cursor-pointer items-center gap-2 px-3 py-1.5 text-xs",
                        focus
                          ? "bg-primary-50 text-primary-700 dark:bg-primary-950 dark:text-primary-400"
                          : "text-slate-700 dark:text-slate-300",
                      )
                    }
                  >
                    <CircleStackIcon className="size-3.5 shrink-0" />
                    <span className="truncate">{v.name}</span>
                  </ListboxOption>
                ))}
              </ListboxOptions>
            </Transition>
          </div>
        </Listbox>
      )}

      {/* Single connection + single vault — just show label */}
      {connections.length === 1 && vaults.length <= 1 && (
        <div className="flex items-center gap-2 px-3 py-1.5 text-xs text-slate-500 dark:text-slate-400">
          <ServerStackIcon className="size-4 shrink-0" />
          <span className="truncate">{activeConnection?.name}</span>
        </div>
      )}
    </div>
  );
}

export { VaultSwitcher };
export type { VaultSwitcherProps };
