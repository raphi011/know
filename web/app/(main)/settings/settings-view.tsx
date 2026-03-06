"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { useTheme } from "@/components/theme-provider";
import type { Theme } from "@/app/lib/types";
import type { ServerConfig } from "@/app/lib/knowhow/types";
import { updateLanguageAction } from "@/app/lib/actions/language";
import { logoutAction } from "@/app/lib/actions/auth";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Separator } from "@/components/ui/separator";

const themes: Theme[] = ["light", "dark", "auto"];

export function SettingsView({
  serverConfig,
}: {
  serverConfig: ServerConfig;
}) {
  const t = useTranslations("settings");
  const { theme, setTheme } = useTheme();
  const [localeError, setLocaleError] = useState<string | null>(null);

  async function handleLocaleChange(locale: "de" | "en") {
    setLocaleError(null);
    const result = await updateLanguageAction(locale);
    if (!result.success) {
      console.error("Language change failed:", result.error);
      setLocaleError(t("languageError"));
      return;
    }
    window.location.reload();
  }

  const isNone = (provider: string) =>
    provider === "none" || provider === "";

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>{t("theme")}</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex gap-2">
            {themes.map((t_) => (
              <Button
                key={t_}
                variant={theme === t_ ? "primary" : "outline"}
                size="sm"
                onClick={() => setTheme(t_)}
              >
                {t(
                  `theme${t_.charAt(0).toUpperCase() + t_.slice(1)}` as
                    | "themeLight"
                    | "themeDark"
                    | "themeAuto",
                )}
              </Button>
            ))}
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t("language")}</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => handleLocaleChange("de")}
            >
              {t("languageDe")}
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => handleLocaleChange("en")}
            >
              {t("languageEn")}
            </Button>
          </div>
          {localeError && (
            <p className="mt-2 text-sm text-red-600" role="alert">
              {localeError}
            </p>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t("serverConfig")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          {/* Capabilities */}
          <div>
            <h3 className="text-sm font-medium text-slate-700 dark:text-slate-300 mb-2">
              {t("capabilities")}
            </h3>
            <div className="flex flex-wrap gap-2">
              <Badge
                variant={
                  serverConfig.semanticSearchEnabled ? "success" : "subtle"
                }
              >
                {t("semanticSearch")}
              </Badge>
              <Badge
                variant={
                  serverConfig.agentChatEnabled ? "success" : "subtle"
                }
              >
                {t("agentChat")}
              </Badge>
              <Badge
                variant={
                  serverConfig.webSearchEnabled ? "success" : "subtle"
                }
              >
                {t("webSearch")}
              </Badge>
            </div>
          </div>

          <div className="border-t border-slate-100 dark:border-slate-800" />

          {/* AI Configuration */}
          <div>
            <h3 className="text-sm font-medium text-slate-700 dark:text-slate-300 mb-2">
              {t("aiConfig")}
            </h3>
            <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
              <dt className="text-slate-500 dark:text-slate-400">
                {t("llmProvider")}
              </dt>
              <dd className="text-slate-900 dark:text-slate-100">
                {isNone(serverConfig.llmProvider)
                  ? t("notConfigured")
                  : serverConfig.llmProvider}
              </dd>
              <dt className="text-slate-500 dark:text-slate-400">
                {t("llmModel")}
              </dt>
              <dd className="text-slate-900 dark:text-slate-100">
                {isNone(serverConfig.llmProvider)
                  ? t("notConfigured")
                  : serverConfig.llmModel}
              </dd>
              <dt className="text-slate-500 dark:text-slate-400">
                {t("embedProvider")}
              </dt>
              <dd className="text-slate-900 dark:text-slate-100">
                {isNone(serverConfig.embedProvider)
                  ? t("notConfigured")
                  : serverConfig.embedProvider}
              </dd>
              <dt className="text-slate-500 dark:text-slate-400">
                {t("embedModel")}
              </dt>
              <dd className="text-slate-900 dark:text-slate-100">
                {isNone(serverConfig.embedProvider)
                  ? t("notConfigured")
                  : serverConfig.embedModel}
              </dd>
              <dt className="text-slate-500 dark:text-slate-400">
                {t("embedDimension")}
              </dt>
              <dd className="text-slate-900 dark:text-slate-100">
                {isNone(serverConfig.embedProvider)
                  ? t("notConfigured")
                  : serverConfig.embedDimension}
              </dd>
            </dl>
          </div>

          <div className="border-t border-slate-100 dark:border-slate-800" />

          {/* Chunking & Versioning */}
          <div>
            <h3 className="text-sm font-medium text-slate-700 dark:text-slate-300 mb-2">
              {t("chunking")}
            </h3>
            <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
              <dt className="text-slate-500 dark:text-slate-400">
                {t("chunkThreshold")}
              </dt>
              <dd className="text-slate-900 dark:text-slate-100">
                {t("chars", { count: serverConfig.chunkThreshold })}
              </dd>
              <dt className="text-slate-500 dark:text-slate-400">
                {t("chunkTargetSize")}
              </dt>
              <dd className="text-slate-900 dark:text-slate-100">
                {t("chars", { count: serverConfig.chunkTargetSize })}
              </dd>
              <dt className="text-slate-500 dark:text-slate-400">
                {t("chunkMinSize")}
              </dt>
              <dd className="text-slate-900 dark:text-slate-100">
                {t("chars", { count: serverConfig.chunkMinSize })}
              </dd>
              <dt className="text-slate-500 dark:text-slate-400">
                {t("chunkMaxSize")}
              </dt>
              <dd className="text-slate-900 dark:text-slate-100">
                {t("chars", { count: serverConfig.chunkMaxSize })}
              </dd>
            </dl>
          </div>

          <div className="border-t border-slate-100 dark:border-slate-800" />

          <div>
            <h3 className="text-sm font-medium text-slate-700 dark:text-slate-300 mb-2">
              {t("versioning")}
            </h3>
            <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
              <dt className="text-slate-500 dark:text-slate-400">
                {t("versionCoalesceMinutes")}
              </dt>
              <dd className="text-slate-900 dark:text-slate-100">
                {t("minutes", {
                  count: serverConfig.versionCoalesceMinutes,
                })}
              </dd>
              <dt className="text-slate-500 dark:text-slate-400">
                {t("versionRetentionCount")}
              </dt>
              <dd className="text-slate-900 dark:text-slate-100">
                {t("versions", {
                  count: serverConfig.versionRetentionCount,
                })}
              </dd>
            </dl>
          </div>
        </CardContent>
      </Card>

      <Separator />

      <form action={logoutAction}>
        <Button type="submit" variant="destructive">
          {t("signOut")}
        </Button>
      </form>
    </div>
  );
}
