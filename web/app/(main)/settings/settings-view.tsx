"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { signOut } from "next-auth/react";
import { useRouter } from "next/navigation";
import { useTheme } from "@/components/theme-provider";
import type { Theme } from "@/app/lib/types";
import { updateLanguageAction } from "@/app/lib/actions/language";
import {
  addConnectionAction,
  removeConnectionAction,
  type ServerConnection,
} from "@/app/lib/actions/connections";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/card";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import {
  ServerStackIcon,
  TrashIcon,
  PlusIcon,
} from "@heroicons/react/24/outline";

const themes: Theme[] = ["light", "dark", "auto"];

export function SettingsView({
  connections,
}: {
  connections: ServerConnection[];
}) {
  const t = useTranslations("settings");
  const { theme, setTheme } = useTheme();
  const router = useRouter();
  const [localeError, setLocaleError] = useState<string | null>(null);
  const [showAddForm, setShowAddForm] = useState(false);
  const [addError, setAddError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

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

  async function handleAddConnection(formData: FormData) {
    setSaving(true);
    setAddError(null);

    const name = formData.get("name") as string;
    const url = formData.get("url") as string;
    const apiToken = formData.get("apiToken") as string;

    const result = await addConnectionAction(name, url, apiToken);
    setSaving(false);

    if (!result.success) {
      setAddError(result.error ?? "Failed to add connection");
      return;
    }

    setShowAddForm(false);
    router.refresh();
  }

  async function handleRemoveConnection(id: string) {
    const result = await removeConnectionAction(id);
    if (!result.success) {
      console.error("Failed to remove connection:", result.error);
    }
    router.refresh();
  }

  return (
    <div className="space-y-4">
      {/* Server Connections */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <ServerStackIcon className="size-5" />
            {t("servers")}
          </CardTitle>
        </CardHeader>
        <CardContent>
          {connections.length === 0 && !showAddForm && (
            <p className="mb-3 text-sm text-slate-500 dark:text-slate-400">
              {t("noServers")}
            </p>
          )}

          {connections.length > 0 && (
            <div className="mb-3 space-y-2">
              {connections.map((conn) => (
                <div
                  key={conn.id}
                  className="flex items-center justify-between rounded-lg border border-slate-200 px-3 py-2 dark:border-slate-700"
                >
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-medium text-slate-900 dark:text-white">
                      {conn.name}
                    </p>
                    <p className="truncate text-xs text-slate-500 dark:text-slate-400">
                      {conn.url}
                    </p>
                  </div>
                  <button
                    onClick={() => handleRemoveConnection(conn.id)}
                    className="ml-2 rounded-lg p-1.5 text-slate-400 hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-950 dark:hover:text-red-400"
                    aria-label={t("removeServer")}
                  >
                    <TrashIcon className="size-4" />
                  </button>
                </div>
              ))}
            </div>
          )}

          {showAddForm ? (
            <form action={handleAddConnection} className="space-y-2">
              <input
                name="name"
                type="text"
                required
                placeholder={t("serverName")}
                className="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm text-slate-900 placeholder:text-slate-400 focus:border-primary-500 focus:outline-none dark:border-slate-700 dark:bg-slate-800 dark:text-white dark:placeholder:text-slate-500"
              />
              <input
                name="url"
                type="url"
                required
                placeholder={t("serverUrl")}
                className="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm text-slate-900 placeholder:text-slate-400 focus:border-primary-500 focus:outline-none dark:border-slate-700 dark:bg-slate-800 dark:text-white dark:placeholder:text-slate-500"
              />
              <input
                name="apiToken"
                type="password"
                required
                placeholder={t("serverToken")}
                className="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm text-slate-900 placeholder:text-slate-400 focus:border-primary-500 focus:outline-none dark:border-slate-700 dark:bg-slate-800 dark:text-white dark:placeholder:text-slate-500"
              />
              {addError && (
                <p className="text-sm text-red-600" role="alert">
                  {addError}
                </p>
              )}
              <div className="flex gap-2">
                <Button type="submit" size="sm" disabled={saving}>
                  {saving ? t("adding") : t("addServer")}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => {
                    setShowAddForm(false);
                    setAddError(null);
                  }}
                >
                  {t("cancel")}
                </Button>
              </div>
            </form>
          ) : (
            <Button
              variant="outline"
              size="sm"
              onClick={() => setShowAddForm(true)}
            >
              <PlusIcon className="mr-1.5 size-4" />
              {t("addServer")}
            </Button>
          )}
        </CardContent>
      </Card>

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

      <Button
        variant="destructive"
        onClick={() => signOut({ callbackUrl: "/login" })}
      >
        {t("signOut")}
      </Button>
    </div>
  );
}
