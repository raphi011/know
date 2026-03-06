import type { Metadata } from "next";
import { getTranslations } from "next-intl/server";
import { PageLayout } from "@/components/page-layout";
import { getServerConfig } from "@/app/lib/knowhow/queries";
import { SettingsView } from "./settings-view";

export async function generateMetadata(): Promise<Metadata> {
  const t = await getTranslations("settings");
  return {
    title: t("title"),
  };
}

export default async function SettingsPage() {
  const [t, serverConfig] = await Promise.all([
    getTranslations("settings"),
    getServerConfig().catch(() => null),
  ]);

  return (
    <PageLayout title={t("title")}>
      <SettingsView serverConfig={serverConfig} />
    </PageLayout>
  );
}
