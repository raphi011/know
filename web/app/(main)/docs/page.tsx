import { redirect } from "next/navigation";
import { getTranslations } from "next-intl/server";
import { getVaults, getVaultDocuments } from "@/app/lib/knowhow/queries";
import { EmptyState } from "@/components/empty-state";
import { DocumentTextIcon } from "@heroicons/react/24/outline";

export default async function DocsPage() {
  const t = await getTranslations("docs");
  const vaults = await getVaults();
  const vault = vaults[0];

  if (!vault) {
    return (
      <EmptyState
        icon={<DocumentTextIcon />}
        title={t("noVaultTitle")}
        description={t("noVaultDescription")}
      />
    );
  }

  const documents = await getVaultDocuments(vault.id);

  if (documents.length > 0) {
    redirect(`/docs/${documents[0]!.path.replace(/^\//, "")}`);
  }

  return (
    <EmptyState
      icon={<DocumentTextIcon />}
      title={t("noDocumentsTitle")}
      description={t("noDocumentsDescription")}
    />
  );
}
