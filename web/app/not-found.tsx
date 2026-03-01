"use client";

import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";

export default function NotFound() {
  const t = useTranslations("notFound");

  return (
    <div className="flex min-h-screen items-center justify-center">
      <div className="text-center">
        <h1 className="text-4xl font-bold text-slate-900 dark:text-white">
          404
        </h1>
        <p className="mt-2 text-lg text-slate-500 dark:text-slate-400">
          {t("title")}
        </p>
        <p className="mt-1 text-sm text-slate-400 dark:text-slate-500">
          {t("description")}
        </p>
        <Button href="/docs" variant="primary" className="mt-6">
          {t("goHome")}
        </Button>
      </div>
    </div>
  );
}
