"use client";

import { useEffect } from "react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";

export default function AuthError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  const t = useTranslations("error");

  useEffect(() => {
    console.error("Auth error:", error);
  }, [error]);

  return (
    <div className="text-center">
      <h2 className="text-xl font-bold text-slate-900 dark:text-white">
        {t("title")}
      </h2>
      <p className="mt-2 text-sm text-slate-500 dark:text-slate-400">
        {t("description")}
      </p>
      {error.digest && (
        <p className="mt-1 text-xs text-slate-400">Reference: {error.digest}</p>
      )}
      <Button onClick={reset} variant="primary" className="mt-4">
        {t("tryAgain")}
      </Button>
    </div>
  );
}
