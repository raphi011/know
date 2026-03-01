import { redirect } from "next/navigation";
import { getCurrentUser } from "@/app/lib/auth";
import { getVaults, getVaultDocuments } from "@/app/lib/knowhow/queries";
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

  const vaults = await getVaults();
  const vault = vaults[0] ?? null;
  const documents = vault ? await getVaultDocuments(vault.id) : [];

  return (
    <AppShellWrapper user={user} vault={vault} documents={documents}>
      {children}
    </AppShellWrapper>
  );
}
