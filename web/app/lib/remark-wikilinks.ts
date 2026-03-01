import type { Root, PhrasingContent, Parent } from "mdast";
import type { Plugin } from "unified";
import { visit } from "unist-util-visit";

const WIKILINK_RE = /\[\[([^\]]+)\]\]/g;

/**
 * Remark plugin that converts `[[target]]` wikilink syntax into link nodes
 * with a `wikilink://` URL scheme. Resolution is deferred to the renderer.
 *
 * Wikilinks inside code spans and fenced blocks are naturally unaffected
 * because their content lives in `code`/`inlineCode` nodes, not `text` nodes.
 */
const remarkWikiLinks: Plugin<[], Root> = () => {
  return (tree) => {
    visit(tree, "text", (node, index, parent: Parent | undefined) => {
      if (!parent || index === undefined) return;

      const value = node.value as string;
      WIKILINK_RE.lastIndex = 0;

      if (!WIKILINK_RE.test(value)) return;

      // Reset and split the text node into text + link segments
      WIKILINK_RE.lastIndex = 0;
      const children: PhrasingContent[] = [];
      let lastIndex = 0;
      let match: RegExpExecArray | null;

      while ((match = WIKILINK_RE.exec(value)) !== null) {
        const target = match[1]!;

        if (match.index > lastIndex) {
          children.push({
            type: "text",
            value: value.slice(lastIndex, match.index),
          });
        }

        if (!target.trim()) {
          children.push({ type: "text", value: match[0] });
        } else {
          children.push({
            type: "link",
            url: `wikilink://${encodeURIComponent(target)}`,
            children: [{ type: "text", value: target }],
          });
        }

        lastIndex = match.index + match[0].length;
      }

      // Remaining text after last match
      if (lastIndex < value.length) {
        children.push({
          type: "text",
          value: value.slice(lastIndex),
        });
      }

      // Replace the original text node with the new children
      parent.children.splice(index, 1, ...children);
    });
  };
};

export { remarkWikiLinks };
