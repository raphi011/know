import { redirect } from "next/navigation";
import { getCurrentUser } from "@/app/lib/auth";
import { getVaults, getVaultDocuments } from "@/app/lib/knowhow/queries";
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
  const authUser = await getCurrentUser();

  if (!authUser && process.env.DISABLE_AUTH !== "1") {
    redirect("/login");
  }

  // Fallback user for local dev with DISABLE_AUTH=1
  const user = authUser ?? { name: "Dev User", email: "dev@localhost" };

  const connections = await getConnections();
  const activeConnection = await getActiveConnection();

  // Only fetch vaults if we have a connection
  let vaults: Awaited<ReturnType<typeof getVaults>> = [];
  if (activeConnection) {
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
  const documents = vault ? await getVaultDocuments(vault.id) : [];

  return (
    <AppShellWrapper
      user={user}
      vault={vault}
      vaults={vaults}
      documents={documents}
      connections={connections}
      activeConnectionId={activeConnection?.id ?? null}
    >
      {children}
    </AppShellWrapper>
  );
}
