"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import type { Heading } from "@/app/lib/extract-headings";
import { cn } from "@/lib/utils";

type DocumentOutlineProps = {
  headings: Heading[];
};

function DocumentOutline({ headings }: DocumentOutlineProps) {
  const t = useTranslations("docs");
  const [activeId, setActiveId] = useState<string | null>(null);
  const observerRef = useRef<IntersectionObserver | null>(null);

  useEffect(() => {
    if (headings.length === 0) return;

    // Track which headings are currently visible
    const visibleHeadings = new Map<string, IntersectionObserverEntry>();

    observerRef.current = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) {
            visibleHeadings.set(entry.target.id, entry);
          } else {
            visibleHeadings.delete(entry.target.id);
          }
        }

        // Pick the topmost visible heading
        if (visibleHeadings.size > 0) {
          let topmost: string | null = null;
          let topY = Infinity;
          for (const [id, entry] of visibleHeadings) {
            if (entry.boundingClientRect.top < topY) {
              topY = entry.boundingClientRect.top;
              topmost = id;
            }
          }
          if (topmost) setActiveId(topmost);
        }
      },
      {
        root: null,
        rootMargin: "0px 0px -80% 0px",
        threshold: 0,
      },
    );

    // Observe all heading elements by ID
    for (const h of headings) {
      const el = document.getElementById(h.id);
      if (el) {
        observerRef.current.observe(el);
      } else if (process.env.NODE_ENV === "development") {
        console.warn(
          `[DocumentOutline] Heading "${h.text}" (id="${h.id}") not found in DOM`,
        );
      }
    }

    return () => {
      observerRef.current?.disconnect();
    };
  }, [headings]);

  if (headings.length === 0) {
    return (
      <p className="px-2 text-sm text-slate-400 dark:text-slate-500">
        {t("noHeadings")}
      </p>
    );
  }

  const minLevel = Math.min(...headings.map((h) => h.level));

  function handleClick(id: string) {
    const el = document.getElementById(id);
    if (el) {
      el.scrollIntoView({ behavior: "smooth", block: "start" });
    }
  }

  return (
    <nav aria-label={t("documentOutline")}>
      <ul className="space-y-0.5">
        {headings.map((heading) => (
          <li key={heading.id}>
            <button
              type="button"
              onClick={() => handleClick(heading.id)}
              className={cn(
                "w-full truncate rounded-md px-2 py-1 text-left text-sm transition-colors",
                "hover:bg-slate-100 dark:hover:bg-slate-800",
                activeId === heading.id
                  ? "font-medium text-primary-600 dark:text-primary-400"
                  : "text-slate-600 dark:text-slate-400",
              )}
              style={{
                paddingLeft: `${(heading.level - minLevel) * 12 + 8}px`,
              }}
            >
              {heading.text}
            </button>
          </li>
        ))}
      </ul>
    </nav>
  );
}

export { DocumentOutline };
