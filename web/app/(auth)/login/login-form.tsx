"use client";

import { useTranslations } from "next-intl";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { loginAction } from "@/app/lib/actions/auth";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/card";

export function LoginForm({
  defaultServerUrl,
}: {
  defaultServerUrl: string;
}) {
  const t = useTranslations("login");
  const router = useRouter();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleSubmit(formData: FormData) {
    setLoading(true);
    setError(null);

    const url = formData.get("url") as string;
    const token = formData.get("token") as string;
    const name = formData.get("name") as string;

    const result = await loginAction(url, token, name);

    if (!result.success) {
      setError(result.error ?? t("error"));
      setLoading(false);
      return;
    }

    router.push("/docs");
  }

  return (
    <Card>
      <CardContent className="mt-0 space-y-6 py-2">
        <div className="text-center">
          <h1 className="text-2xl font-bold text-slate-900 dark:text-white">
            {t("title")}
          </h1>
          <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">
            {t("subtitle")}
          </p>
        </div>

        {error && (
          <p className="text-center text-sm text-red-600 dark:text-red-400">
            {error}
          </p>
        )}

        <form action={handleSubmit} className="space-y-3">
          <input
            name="url"
            type="url"
            required
            placeholder={t("serverUrl")}
            defaultValue={defaultServerUrl}
            className="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm text-slate-900 placeholder:text-slate-400 focus:border-primary-500 focus:outline-none dark:border-slate-700 dark:bg-slate-800 dark:text-white dark:placeholder:text-slate-500"
          />
          <input
            name="token"
            type="password"
            required
            placeholder={t("apiToken")}
            className="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm text-slate-900 placeholder:text-slate-400 focus:border-primary-500 focus:outline-none dark:border-slate-700 dark:bg-slate-800 dark:text-white dark:placeholder:text-slate-500"
          />
          <input
            name="name"
            type="text"
            placeholder={t("serverName")}
            className="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm text-slate-900 placeholder:text-slate-400 focus:border-primary-500 focus:outline-none dark:border-slate-700 dark:bg-slate-800 dark:text-white dark:placeholder:text-slate-500"
          />
          <Button type="submit" loading={loading} className="w-full">
            {loading ? t("connecting") : t("connect")}
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}
