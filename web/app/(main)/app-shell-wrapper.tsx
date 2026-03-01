"use client";

import { useEffect, useState } from "react";
import { usePathname, useRouter } from "next/navigation";
import { AppShell } from "@/components/app-shell";
import { DocSidebar } from "@/components/domain/doc-sidebar";
import { DocumentsProvider } from "@/components/domain/documents-context";
import { SearchCommandPalette } from "@/components/domain/search-command-palette";
import { VaultSwitcher } from "@/components/domain/vault-switcher";
import { buildTree } from "@/app/lib/knowhow/tree";
import type { Vault, DocumentSummary, ServerConnection } from "@/app/lib/knowhow/types";

type User = {
  id?: string;
  name?: string | null;
  email?: string | null;
  image?: string | null;
};

export function AppShellWrapper({
  user,
  vault,
  vaults,
  documents,
  connections,
  activeConnectionId,
  children,
}: {
  user: User;
  vault: Vault | null;
  vaults: Vault[];
  documents: DocumentSummary[];
  connections: ServerConnection[];
  activeConnectionId: string | null;
  children: React.ReactNode;
}) {
  const router = useRouter();
  const pathname = usePathname();
  const [searchOpen, setSearchOpen] = useState(false);

  const tree = buildTree(documents);

  // Global Cmd+K / Ctrl+K shortcut
  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === "k" && (e.metaKey || e.ctrlKey)) {
        e.preventDefault();
        if (vault) setSearchOpen(true);
      }
    }
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [vault]);

  const sidebarContent = (
    <>
      <VaultSwitcher
        connections={connections}
        activeConnectionId={activeConnectionId}
        vaults={vaults}
        activeVaultId={vault?.id ?? null}
      />
      {vault && <DocSidebar tree={tree} />}
    </>
  );

  return (
    <DocumentsProvider documents={documents}>
      <AppShell
        appName="Knowhow"
        navSections={[]}
        sidebarContent={sidebarContent}
        profile={{
          name: user.name ?? user.email ?? "User",
          avatarSrc: user.image,
          href: "/settings",
        }}
        activeHref={pathname}
        onNavigate={(href) => router.push(href)}
        onSearchClick={vault ? () => setSearchOpen(true) : undefined}
      >
        {children}
      </AppShell>

      {vault && (
        <SearchCommandPalette
          vaultId={vault.id}
          open={searchOpen}
          onClose={() => setSearchOpen(false)}
        />
      )}
    </DocumentsProvider>
  );
}
