"use client";

import { useTranslations } from "next-intl";
import { signIn } from "next-auth/react";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/card";

export default function LoginPage() {
  const t = useTranslations("login");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleSignIn() {
    setLoading(true);
    setError(null);
    try {
      await signIn("oidc", { callbackUrl: "/docs" });
    } catch (err) {
      console.error("Sign-in failed:", err);
      setError(t("error"));
      setLoading(false);
    }
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

        <Button onClick={handleSignIn} loading={loading} className="w-full">
          {loading ? t("signingIn") : t("signIn")}
        </Button>
      </CardContent>
    </Card>
  );
}
