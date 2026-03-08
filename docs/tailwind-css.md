# Tailwind CSS

[Tailwind CSS](https://tailwindcss.com/) is used for all styling. Version 4 with CSS-native configuration — no `tailwind.config.js`, no Node.js.

## Setup

### Config via CSS

**File**: `internal/web/static/css/input.css`

```css
@import "tailwindcss";

@theme {
  --color-primary-50: #eef2ff;
  --color-primary-500: #6366f1;
  /* ... full palette ... */
  --color-surface: #ffffff;
  --color-surface-dark: #0f172a;
  --color-border: #e2e8f0;
  --color-border-dark: #334155;
  --color-muted: #64748b;
}

@variant dark (&:where(.dark, .dark *));

@layer base {
  body { /* ... */ }
}
```

Tailwind v4 uses `@theme` blocks in CSS instead of a JS config file. All color tokens are defined here as CSS custom properties.

### Build

```bash
just css         # One-time build
just css-watch   # Watch mode for development
just generate-all  # Full rebuild (gqlgen + templ + CSS)
```

Uses the standalone Tailwind CLI (`/opt/homebrew/bin/tailwindcss`), no npm/Node.js:

```
input.css → tailwindcss CLI → app.css (3,245 lines, committed)
```

### Inclusion

```html
<!-- layout.templ -->
<link rel="stylesheet" href="/static/css/app.css"/>
```

Served from embedded static assets via Go's `embed.FS`.

## Semantic Color Tokens

Use semantic token names, not raw Tailwind colors:

| Token | Light | Dark | Usage |
|-------|-------|------|-------|
| `primary-*` | Indigo scale | Same | Buttons, links, active states |
| `accent-*` | Amber scale | Same | Highlights, badges |
| `surface` | White | Slate 950 | Page/card backgrounds |
| `border` | Slate 200 | Slate 700 | Borders, dividers |
| `muted` | Slate 500 | Slate 400 | Secondary text |

```html
<!-- Good: semantic tokens -->
<div class="bg-surface text-muted border-border">

<!-- Avoid: raw colors for themed elements -->
<div class="bg-white text-gray-500 border-gray-200">
```

For the full color palette, see `docs/design-system.md`.

## Dark Mode

Class-based dark mode using `@variant dark`:

```go
// layout.templ — toggles dark class on <html>
<html class={ templ.KV("dark", data.Theme == "dark") }>
```

```html
<!-- Dual-mode styling -->
<div class="bg-surface dark:bg-surface-dark border-border dark:border-border-dark">
<p class="text-gray-700 dark:text-gray-300">
```

Theme is stored in the `kh_theme` cookie and applied before first paint in `app.js` to prevent FOUC (Flash of Unstyled Content).

## Common Patterns

### Layout

```html
<!-- Two-column: sidebar + content -->
<div class="flex gap-6">
    <aside class="w-64 shrink-0">...</aside>
    <div class="min-w-0 flex-1">...</div>
</div>

<!-- Full-height with scroll -->
<div class="flex h-[calc(100vh-8rem)] flex-col">
    <div class="flex-1 overflow-y-auto">...</div>
    <div><!-- sticky footer/input --></div>
</div>
```

### Responsive

```html
<div class="px-4 sm:px-6 lg:px-8">  <!-- Progressive padding -->
<div class="max-w-7xl mx-auto">     <!-- Centered content -->
```

Minimal responsive breakpoints — the app is primarily desktop-focused.

### Prose (Markdown)

```html
<article class="prose prose-sm dark:prose-invert max-w-none">
    @templ.Raw(renderedMarkdown)
</article>
```

`prose` applies typography styles to rendered markdown. `max-w-none` removes the default max-width constraint.

### Buttons

```html
<!-- Primary -->
<button class="bg-primary-600 hover:bg-primary-700 text-white rounded-md px-3 py-1.5 text-sm font-medium">

<!-- Ghost/link-style -->
<button class="text-primary-600 hover:text-primary-700 text-sm">
```

### Form Inputs

```html
<input class="w-full rounded-md border border-border dark:border-border-dark bg-surface dark:bg-surface-dark px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary-500"/>
```

## Conventions

1. **Semantic tokens for colors** — use `primary-*`, `surface`, `border`, `muted` from `@theme`; avoid raw `gray-*` for themed elements
2. **No CSS-in-JS** — all styling via Tailwind utility classes in templ files
3. **No custom component classes** — use Tailwind utilities directly; extract templ components instead of CSS components
4. **Dark mode on every visual element** — always add `dark:` variants for backgrounds, borders, and text colors
5. **`app.css` is committed** — rebuild with `just css` after changing `input.css` or adding new utility classes
6. **Minimal custom CSS** — only `@theme` tokens and `@layer base` resets in `input.css`
