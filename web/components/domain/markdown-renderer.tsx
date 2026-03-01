"use client";

import Link from "next/link";
import ReactMarkdown, { defaultUrlTransform } from "react-markdown";
import remarkGfm from "remark-gfm";
import remarkFrontmatter from "remark-frontmatter";
import rehypeHighlight from "rehype-highlight";
import { remarkWikiLinks } from "@/app/lib/remark-wikilinks";
import { useDocuments } from "@/components/domain/documents-context";
import { resolveWikiLink } from "@/app/lib/knowhow/resolve-wikilink";
import { cn } from "@/lib/utils";

const WIKILINK_PREFIX = "wikilink://";

type MarkdownRendererProps = {
  content: string;
  className?: string;
};

function WikiLinkAwareLink(
  props: React.AnchorHTMLAttributes<HTMLAnchorElement>,
) {
  const { href, children, ...rest } = props;
  const documents = useDocuments();

  if (href?.startsWith(WIKILINK_PREFIX)) {
    const target = decodeURIComponent(href.slice(WIKILINK_PREFIX.length));
    const resolved = resolveWikiLink(target, documents);

    if (resolved) {
      return (
        <Link
          href={`/docs/${resolved.path.replace(/^\//, "")}`}
          className="text-primary-600 underline decoration-primary-300 hover:text-primary-500 hover:decoration-primary-400 dark:decoration-primary-700"
          title={resolved.title}
        >
          {children}
        </Link>
      );
    }

    // Dangling link — unresolved target
    return (
      <span
        className="text-red-500 underline decoration-dashed cursor-not-allowed"
        title={`Page not found: ${target}`}
      >
        {children}
      </span>
    );
  }

  // Regular link — pass through
  return (
    <a href={href} {...rest}>
      {children}
    </a>
  );
}

function MarkdownRenderer({ content, className }: MarkdownRendererProps) {
  return (
    <div
      className={cn(
        "prose prose-slate mx-auto dark:prose-invert",
        // Typora-like sizing: comfortable reading width, generous text
        "prose-lg max-w-[48rem]",
        // Headings — larger weight, bottom border on h2
        "prose-h1:text-3xl prose-h1:font-bold prose-h1:mb-6",
        "prose-h2:text-2xl prose-h2:font-bold prose-h2:border-b prose-h2:border-slate-200 prose-h2:pb-2 dark:prose-h2:border-slate-700",
        "prose-h3:text-xl prose-h3:font-semibold",
        // Paragraph spacing
        "prose-p:leading-relaxed",
        // Links — use app primary tokens
        "prose-a:text-primary-600 prose-a:underline prose-a:decoration-primary-300 hover:prose-a:text-primary-500 hover:prose-a:decoration-primary-400",
        "dark:prose-a:decoration-primary-700",
        // Inline code
        "prose-code:rounded prose-code:bg-slate-100 prose-code:px-1.5 prose-code:py-0.5 prose-code:text-sm prose-code:font-normal prose-code:text-slate-700 prose-code:before:content-none prose-code:after:content-none",
        "dark:prose-code:bg-slate-800 dark:prose-code:text-slate-300",
        // Code blocks — visible background to set apart from content
        "prose-pre:rounded-lg prose-pre:bg-slate-100 dark:prose-pre:bg-slate-800/80",
        "[&_.hljs]:bg-transparent [&_.hljs]:border-0",
        // Blockquotes — subtle left border
        "prose-blockquote:border-l-slate-300 prose-blockquote:text-slate-600 prose-blockquote:font-normal dark:prose-blockquote:border-l-slate-600 dark:prose-blockquote:text-slate-400",
        className,
      )}
    >
      <ReactMarkdown
        urlTransform={(url) => {
          if (url.startsWith("wikilink://")) return url;
          return defaultUrlTransform(url);
        }}
        remarkPlugins={[remarkFrontmatter, remarkGfm, remarkWikiLinks]}
        rehypePlugins={[rehypeHighlight]}
        components={{ a: WikiLinkAwareLink }}
      >
        {content}
      </ReactMarkdown>
    </div>
  );
}

export { MarkdownRenderer };
