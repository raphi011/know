"use client";

import Link from "next/link";
import { useTranslations, useFormatter } from "next-intl";
import { Badge } from "@/components/ui/badge";
import { useDocuments } from "@/components/domain/documents-context";
import { resolveWikiLink } from "@/app/lib/knowhow/resolve-wikilink";
import { routes } from "@/app/lib/routes";
import type { Document } from "@/app/lib/knowhow/types";
import { cn } from "@/lib/utils";

type DocumentInfoProps = {
  document: Document;
};

function DocumentInfo({ document }: DocumentInfoProps) {
  const t = useTranslations("docs");
  const format = useFormatter();
  const documents = useDocuments();

  return (
    <div className="space-y-5">
      {/* Labels */}
      <Section title={t("labels")}>
        {document.labels.length > 0 ? (
          <div className="flex flex-wrap gap-1.5">
            {document.labels.map((label) => (
              <Badge key={label} variant="subtle">
                {label}
              </Badge>
            ))}
          </div>
        ) : (
          <EmptyText>—</EmptyText>
        )}
      </Section>

      {/* Wiki-links (outgoing) */}
      <Section title={t("wikiLinks")}>
        {document.wikiLinks.length > 0 ? (
          <ul className="space-y-1">
            {document.wikiLinks.map((link) => {
              const resolved = link.resolved
                ? resolveWikiLink(link.rawTarget, documents)
                : null;
              return (
                <li key={link.id}>
                  {resolved ? (
                    <Link
                      href={`${routes.docs}${resolved.path}`}
                      className="text-sm text-primary-600 hover:underline dark:text-primary-400"
                    >
                      {resolved.title}
                    </Link>
                  ) : (
                    <span className="text-sm text-slate-400 dark:text-slate-500">
                      {link.rawTarget}{" "}
                      <span className="text-xs">({t("unresolvedLink")})</span>
                    </span>
                  )}
                </li>
              );
            })}
          </ul>
        ) : (
          <EmptyText>{t("noLinks")}</EmptyText>
        )}
      </Section>

      {/* Backlinks (incoming) */}
      <Section title={t("backlinks")}>
        {document.backlinks.length > 0 ? (
          <ul className="space-y-1">
            {document.backlinks.map((link) => {
              const source = documents.find((d) => d.id === link.fromDocId);
              return (
                <li key={link.id}>
                  {source ? (
                    <Link
                      href={`${routes.docs}${source.path}`}
                      className="text-sm text-primary-600 hover:underline dark:text-primary-400"
                    >
                      {source.title}
                    </Link>
                  ) : (
                    <span className="text-sm text-slate-400 dark:text-slate-500">
                      {link.rawTarget}
                    </span>
                  )}
                </li>
              );
            })}
          </ul>
        ) : (
          <EmptyText>{t("noBacklinks")}</EmptyText>
        )}
      </Section>

      {/* Details */}
      <Section title={t("details")}>
        <dl className="space-y-2 text-sm">
          {document.docType && (
            <DetailRow label={t("docType")} value={document.docType} />
          )}
          <DetailRow
            label={t("createdAt")}
            value={format.dateTime(new Date(document.createdAt), {
              dateStyle: "medium",
            })}
          />
          <DetailRow
            label={t("updatedAt")}
            value={format.dateTime(new Date(document.updatedAt), {
              dateStyle: "medium",
            })}
          />
        </dl>
      </Section>
    </div>
  );
}

function Section({
  title,
  children,
}: {
  title: string;
  children: React.ReactNode;
}) {
  return (
    <div>
      <h3 className="mb-2 text-xs font-semibold uppercase tracking-wider text-slate-400 dark:text-slate-500">
        {title}
      </h3>
      {children}
    </div>
  );
}

function DetailRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex justify-between gap-2">
      <dt className="text-slate-500 dark:text-slate-400">{label}</dt>
      <dd className="text-right text-slate-700 dark:text-slate-300">
        {value}
      </dd>
    </div>
  );
}

function EmptyText({
  children,
  className,
}: {
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <p className={cn("text-sm text-slate-400 dark:text-slate-500", className)}>
      {children}
    </p>
  );
}

export { DocumentInfo };
