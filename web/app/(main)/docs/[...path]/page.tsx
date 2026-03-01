import { notFound } from "next/navigation";
import { getVaults, getDocument } from "@/app/lib/knowhow/queries";
import { DocumentEditor } from "@/components/domain/document-editor";

type Props = {
  params: Promise<{ path: string[] }>;
};

export default async function DocumentPage({ params }: Props) {
  const { path } = await params;
  const docPath = "/" + path.join("/");

  const vaults = await getVaults();
  const vault = vaults[0];

  if (!vault) {
    notFound();
  }

  const document = await getDocument(vault.id, docPath);

  if (!document) {
    notFound();
  }

  return <DocumentEditor document={document} vaultId={vault.id} />;
}
