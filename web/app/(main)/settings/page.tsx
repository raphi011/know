import type { Metadata } from "next";
import { getTranslations } from "next-intl/server";
import { PageLayout } from "@/components/page-layout";
import { getConnections } from "@/app/lib/actions/connections";
import { SettingsView } from "./settings-view";

export async function generateMetadata(): Promise<Metadata> {
  const t = await getTranslations("settings");
  return {
    title: t("title"),
  };
}

export default async function SettingsPage() {
  const t = await getTranslations("settings");
  const connections = await getConnections();

  return (
    <PageLayout title={t("title")}>
      <SettingsView connections={connections} />
    </PageLayout>
  );
}
