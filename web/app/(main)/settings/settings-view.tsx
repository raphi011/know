"use client";

import type { ReactNode } from "react";
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

function SectionHeader({ children }: { children: ReactNode }) {
  return (
    <h3 className="text-sm font-medium text-slate-700 dark:text-slate-300 mb-2">
      {children}
    </h3>
  );
}

function ConfigRow({ label, value }: { label: string; value: ReactNode }) {
  return (
    <>
      <dt className="text-slate-500 dark:text-slate-400">{label}</dt>
      <dd className="text-slate-900 dark:text-slate-100">{value}</dd>
    </>
  );
}

function isNone(provider: string) {
  return provider === "none" || provider === "";
}

export function SettingsView({
  serverConfig,
}: {
  serverConfig: ServerConfig | null;
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

  const capabilities = [
    { key: "semanticSearch", enabled: serverConfig?.semanticSearchEnabled },
    { key: "agentChat", enabled: serverConfig?.agentChatEnabled },
    { key: "webSearch", enabled: serverConfig?.webSearchEnabled },
  ] as const;

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
          {serverConfig === null ? (
            <p className="text-sm text-red-600" role="alert">
              {t("configError")}
            </p>
          ) : (
            <>
              <div>
                <SectionHeader>{t("capabilities")}</SectionHeader>
                <div className="flex flex-wrap gap-2">
                  {capabilities.map(({ key, enabled }) => (
                    <Badge
                      key={key}
                      variant={enabled ? "success" : "subtle"}
                    >
                      {t(key)}
                    </Badge>
                  ))}
                </div>
              </div>

              <Separator />

              <div>
                <SectionHeader>{t("aiConfig")}</SectionHeader>
                <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
                  <ConfigRow
                    label={t("llmProvider")}
                    value={
                      isNone(serverConfig.llmProvider)
                        ? t("notConfigured")
                        : serverConfig.llmProvider
                    }
                  />
                  <ConfigRow
                    label={t("llmModel")}
                    value={
                      isNone(serverConfig.llmProvider)
                        ? t("notConfigured")
                        : serverConfig.llmModel
                    }
                  />
                  <ConfigRow
                    label={t("embedProvider")}
                    value={
                      isNone(serverConfig.embedProvider)
                        ? t("notConfigured")
                        : serverConfig.embedProvider
                    }
                  />
                  <ConfigRow
                    label={t("embedModel")}
                    value={
                      isNone(serverConfig.embedProvider)
                        ? t("notConfigured")
                        : serverConfig.embedModel
                    }
                  />
                  <ConfigRow
                    label={t("embedDimension")}
                    value={
                      isNone(serverConfig.embedProvider)
                        ? t("notConfigured")
                        : serverConfig.embedDimension
                    }
                  />
                </dl>
              </div>

              <Separator />

              <div>
                <SectionHeader>{t("chunking")}</SectionHeader>
                <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
                  <ConfigRow
                    label={t("chunkThreshold")}
                    value={t("chars", { count: serverConfig.chunkThreshold })}
                  />
                  <ConfigRow
                    label={t("chunkTargetSize")}
                    value={t("chars", { count: serverConfig.chunkTargetSize })}
                  />
                  <ConfigRow
                    label={t("chunkMinSize")}
                    value={t("chars", { count: serverConfig.chunkMinSize })}
                  />
                  <ConfigRow
                    label={t("chunkMaxSize")}
                    value={t("chars", { count: serverConfig.chunkMaxSize })}
                  />
                </dl>
              </div>

              <Separator />

              <div>
                <SectionHeader>{t("versioning")}</SectionHeader>
                <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
                  <ConfigRow
                    label={t("versionCoalesceMinutes")}
                    value={t("minutes", {
                      count: serverConfig.versionCoalesceMinutes,
                    })}
                  />
                  <ConfigRow
                    label={t("versionRetentionCount")}
                    value={t("versions", {
                      count: serverConfig.versionRetentionCount,
                    })}
                  />
                </dl>
              </div>
            </>
          )}
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
