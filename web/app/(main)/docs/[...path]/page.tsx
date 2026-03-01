import { notFound } from "next/navigation";
import { getVaults, getDocument } from "@/app/lib/knowhow/queries";
import { getActiveVaultId } from "@/app/lib/actions/connections";
import { DocumentEditor } from "@/components/domain/document-editor";

type Props = {
  params: Promise<{ path: string[] }>;
};

export default async function DocumentPage({ params }: Props) {
  const { path } = await params;
  const docPath = "/" + path.join("/");

  const vaults = await getVaults();
  const activeVaultId = await getActiveVaultId();
  const vault = vaults.find((v) => v.id === activeVaultId) ?? vaults[0];

  if (!vault) {
    notFound();
  }

  const document = await getDocument(vault.id, docPath);

  if (!document) {
    notFound();
  }

  return <DocumentEditor document={document} vaultId={vault.id} />;
}
