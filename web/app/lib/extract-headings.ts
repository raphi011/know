import GithubSlugger from "github-slugger";

export type HeadingLevel = 1 | 2 | 3 | 4 | 5 | 6;

export type Heading = {
  level: HeadingLevel;
  text: string;
  id: string;
};

/** Strip inline markdown syntax so slugs match rehype-slug output. */
function stripInlineMarkdown(text: string): string {
  return text
    .replace(/\*\*(.+?)\*\*/g, "$1")
    .replace(/__(.+?)__/g, "$1")
    .replace(/\*(.+?)\*/g, "$1")
    .replace(/_(.+?)_/g, "$1")
    .replace(/`(.+?)`/g, "$1")
    .replace(/\[(.+?)\]\(.+?\)/g, "$1")
    .replace(/~~(.+?)~~/g, "$1")
    .trim();
}

/**
 * Extract headings from raw markdown, generating IDs that match
 * rehype-slug's output (both use github-slugger under the hood).
 */
export function extractHeadings(markdown: string): Heading[] {
  const slugger = new GithubSlugger();
  const headings: Heading[] = [];
  const lines = markdown.split("\n");
  let inCodeBlock = false;

  for (const line of lines) {
    if (line.trimStart().startsWith("```")) {
      inCodeBlock = !inCodeBlock;
      continue;
    }
    if (inCodeBlock) continue;

    const match = line.match(/^(#{1,6})\s+(.+)$/);
    if (match?.[1] && match[2]) {
      const level = match[1].length as HeadingLevel;
      const text = stripInlineMarkdown(match[2]);
      headings.push({ level, text, id: slugger.slug(text) });
    }
  }

  return headings;
}
