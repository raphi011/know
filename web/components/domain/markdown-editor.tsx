"use client";

import { useRef, useEffect } from "react";
import { useTheme } from "@/components/theme-provider";
import { EditorState, Compartment } from "@codemirror/state";
import {
  EditorView,
  keymap,
  lineNumbers,
  drawSelection,
} from "@codemirror/view";
import { markdown } from "@codemirror/lang-markdown";
import { languages } from "@codemirror/language-data";
import { defaultKeymap, history, historyKeymap } from "@codemirror/commands";
import { searchKeymap, highlightSelectionMatches } from "@codemirror/search";
import { oneDark } from "@codemirror/theme-one-dark";
import {
  syntaxHighlighting,
  defaultHighlightStyle,
  bracketMatching,
} from "@codemirror/language";
import {
  autocompletion,
  closeBrackets,
  closeBracketsKeymap,
  completionKeymap,
} from "@codemirror/autocomplete";

type MarkdownEditorProps = {
  content: string;
  onChange: (content: string) => void;
  readOnly?: boolean;
};

const lightTheme = EditorView.theme({
  "&": {
    backgroundColor: "white",
    color: "#1e293b",
  },
  ".cm-gutters": {
    backgroundColor: "#f8fafc",
    borderRight: "1px solid #e2e8f0",
    color: "#94a3b8",
  },
  ".cm-activeLineGutter": {
    backgroundColor: "#f1f5f9",
  },
  ".cm-activeLine": {
    backgroundColor: "#f8fafc",
  },
  "&.cm-focused .cm-cursor": {
    borderLeftColor: "#1e293b",
  },
  "&.cm-focused .cm-selectionBackground, .cm-selectionBackground": {
    backgroundColor: "#dbeafe",
  },
});

function MarkdownEditor({ content, onChange, readOnly }: MarkdownEditorProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const viewRef = useRef<EditorView | null>(null);
  const themeCompartment = useRef(new Compartment());
  const readOnlyCompartment = useRef(new Compartment());
  const { theme } = useTheme();

  function isDarkMode() {
    if (theme === "dark") return true;
    if (theme === "light") return false;
    return (
      typeof window !== "undefined" &&
      window.matchMedia("(prefers-color-scheme: dark)").matches
    );
  }

  // Create editor on mount
  useEffect(() => {
    if (!containerRef.current) return;

    const isDark = isDarkMode();

    const state = EditorState.create({
      doc: content,
      extensions: [
        lineNumbers(),
        history(),
        drawSelection(),
        bracketMatching(),
        closeBrackets(),
        autocompletion(),
        highlightSelectionMatches(),
        syntaxHighlighting(defaultHighlightStyle, { fallback: true }),
        markdown({ codeLanguages: languages }),
        keymap.of([
          ...defaultKeymap,
          ...historyKeymap,
          ...searchKeymap,
          ...closeBracketsKeymap,
          ...completionKeymap,
        ]),
        themeCompartment.current.of(isDark ? oneDark : lightTheme),
        readOnlyCompartment.current.of(
          EditorState.readOnly.of(readOnly ?? false),
        ),
        EditorView.updateListener.of((update) => {
          if (update.docChanged) {
            onChange(update.state.doc.toString());
          }
        }),
        EditorView.theme({
          "&": {
            height: "100%",
            fontSize: "14px",
          },
          ".cm-scroller": {
            fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace",
            overflow: "auto",
          },
          ".cm-content": {
            padding: "8px 0",
          },
        }),
      ],
    });

    const view = new EditorView({
      state,
      parent: containerRef.current,
    });

    viewRef.current = view;

    return () => {
      view.destroy();
      viewRef.current = null;
    };
    // Only run on mount — content/onChange are initial values
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Reconfigure theme when it changes
  useEffect(() => {
    const view = viewRef.current;
    if (!view) return;

    const isDark = isDarkMode();
    view.dispatch({
      effects: themeCompartment.current.reconfigure(
        isDark ? oneDark : lightTheme,
      ),
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [theme]);

  // Reconfigure readOnly when it changes
  useEffect(() => {
    const view = viewRef.current;
    if (!view) return;

    view.dispatch({
      effects: readOnlyCompartment.current.reconfigure(
        EditorState.readOnly.of(readOnly ?? false),
      ),
    });
  }, [readOnly]);

  return (
    <div
      ref={containerRef}
      className="h-full overflow-hidden rounded-lg border border-slate-200 dark:border-slate-800"
    />
  );
}

export { MarkdownEditor };
