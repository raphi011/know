"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { useTheme } from "@/components/theme-provider";
import type { Theme } from "@/app/lib/types";
import { updateLanguageAction } from "@/app/lib/actions/language";
import { logoutAction } from "@/app/lib/actions/auth";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/card";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";

const themes: Theme[] = ["light", "dark", "auto"];

export function SettingsView() {
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

      <Separator />

      <form action={logoutAction}>
        <Button type="submit" variant="destructive">
          {t("signOut")}
        </Button>
      </form>
    </div>
  );
}
