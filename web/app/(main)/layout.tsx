import { redirect } from "next/navigation";
import { getSession } from "@/app/lib/session";
import { env } from "@/app/lib/env";
import { getVaults, getVaultDocuments, getVaultFolders } from "@/app/lib/knowhow/queries";
import {
  getConnections,
  getActiveConnection,
  getActiveVaultId,
} from "@/app/lib/actions/connections";
import { AppShellWrapper } from "./app-shell-wrapper";

export default async function MainLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  let connections: Awaited<ReturnType<typeof getConnections>> = [];
  let activeConnection: Awaited<ReturnType<typeof getActiveConnection>> = null;

  if (!env.AUTH_DISABLED) {
    const session = await getSession();
    if (!session || session.servers.length === 0) {
      redirect("/login");
    }
    connections = await getConnections();
    activeConnection = await getActiveConnection();
  }

  // Fetch vaults — in no-auth mode gql() uses BACKEND_URL directly
  let vaults: Awaited<ReturnType<typeof getVaults>> = [];
  if (env.AUTH_DISABLED || activeConnection) {
    try {
      vaults = await getVaults();
    } catch {
      // Connection may be unreachable — show empty state
    }
  }

  // Determine active vault
  const activeVaultId = await getActiveVaultId();
  const vault =
    vaults.find((v) => v.id === activeVaultId) ?? vaults[0] ?? null;
  const [documents, folders] = vault
    ? await Promise.all([getVaultDocuments(vault.id), getVaultFolders(vault.id)])
    : [[], []];

  return (
    <AppShellWrapper
      vault={vault}
      vaults={vaults}
      documents={documents}
      folderPaths={folders.map((f) => f.path)}
      connections={connections}
      activeConnectionId={activeConnection?.id ?? null}
    >
      {children}
    </AppShellWrapper>
  );
}
