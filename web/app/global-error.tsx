"use client";

import { useEffect } from "react";

// global-error replaces the entire <html>, so no i18n provider is available
export default function GlobalError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    console.error("Unhandled error:", error);
  }, [error]);

  return (
    <html>
      <body>
        <div className="flex min-h-screen items-center justify-center">
          <div className="text-center">
            <h1 className="text-4xl font-bold text-slate-900">
              Something went wrong
            </h1>
            <p className="mt-2 text-slate-500">An unexpected error occurred.</p>
            {error.digest && (
              <p className="mt-1 text-xs text-slate-400">
                Reference: {error.digest}
              </p>
            )}
            <button
              onClick={reset}
              className="mt-6 rounded-xl bg-primary-500 px-6 py-2.5 text-sm font-semibold text-white hover:bg-primary-600"
            >
              Try again
            </button>
          </div>
        </div>
      </body>
    </html>
  );
}
