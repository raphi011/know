# React & Next.js Best Practices 2025/2026

> Reference document for auditing the pyramid codebase against current React ecosystem conventions.
> Each section includes an **Audit checklist** for comparison.

---

## 1. Server Components as Default

React Server Components (RSC) are the default in Next.js App Router. Components render on the server unless explicitly marked `"use client"`.

### Key Principles

- **Server-first**: Keep data fetching, heavy computation, and sensitive logic in Server Components
- **Minimize `"use client"`**: Only add the directive for components that need interactivity (event handlers, hooks like `useState`, `useEffect`, browser APIs)
- **Push `"use client"` to the leaves**: Instead of marking a whole page as client, extract only the interactive part into a small Client Component
- **Container/Presentational split**: Server Component fetches data -> passes props to Client Component for rendering
- **No serialization of functions**: Props passed from Server -> Client must be serializable (no callbacks, class instances)

### Streaming & Suspense

- Use `<Suspense>` boundaries to stream partial UI — users see the shell instantly while data loads
- Don't over-granularize Suspense — group related data fetches under one boundary to avoid choppy loading states
- Use `loading.tsx` route-level files for page-level loading states

### `<Activity>` Component (React 19.2)

Manages offscreen content without destroying state.

```tsx
<Activity mode={isVisible ? "visible" : "hidden"}>
  <ExpensiveComponent />
</Activity>
```

- **`visible`**: Normal rendering
- **`hidden`**: Children rendered with `display: none`, effects unmounted, updates deferred
- Use cases: preserving tab state during navigation, pre-rendering upcoming views, keeping scroll position when switching panels
- Replaces manual "keep mounted but hidden" patterns with a framework primitive

### View Transitions (React 19.2)

```tsx
import { ViewTransition } from 'react'

<ViewTransition>
  <PageContent />
</ViewTransition>
```

- Animates elements during `startTransition` or navigation
- Built on the browser's View Transition API
- Use for page transitions, list reordering, and layout animations

### Audit Checklist

- [ ] Components without interactivity are Server Components (no `"use client"`)
- [ ] `"use client"` is pushed as low in the tree as possible
- [ ] Data fetching happens in Server Components, not in `useEffect`
- [ ] Suspense boundaries exist for async data, not wrapping every component
- [ ] No `window`/`document` access outside `useEffect` in client components
- [ ] Consider `<Activity>` for preserving offscreen component state

---

## 2. React 19 Actions & Form Handling

React 19 introduces Actions — async functions that handle form submissions and mutations natively.

### New Hooks

| Hook | Purpose |
|------|---------|
| `useActionState` | Wraps an Action, returns `[state, formAction, isPending]` — tracks result + pending state |
| `useFormStatus` | Reads the parent `<form>`'s submission state (pending, data, method) — use in submit button components |
| `useOptimistic` | Shows optimistic UI instantly while the server processes — auto-reverts on error |
| `useEffectEvent` | Extracts non-reactive logic from Effects — the returned function should NOT be in the dependency array |

### `useEffectEvent` (React 19.2)

```tsx
function ChatRoom({ roomId, theme }) {
  const onMessage = useEffectEvent((message) => {
    // Reads `theme` without causing the effect to re-run when theme changes
    showNotification(message, theme)
  })

  useEffect(() => {
    const conn = createConnection(roomId)
    conn.on('message', onMessage)
    return () => conn.disconnect()
  }, [roomId])  // No need to include `onMessage` or `theme`
}
```

- Solves the "stale closure" problem without adding values to the dependency array
- The function always reads the latest values but doesn't trigger effect re-runs
- Replaces the common `useCallback` + ref workaround pattern

### Server Actions

- Defined with `"use server"` directive — execute on the server, callable from client
- When passed to `<form action={serverAction}>`, forms are **progressively enhanced** (work without JS)
- Single active mutation per form — no double-submit risk
- Use `useActionState` for form state + validation errors returned from the server

### Best Practices

- **Prefer `<form action={...}>` over `onClick` handlers** for mutations — gets progressive enhancement for free
- **Use `useFormStatus` in a child component** of the form (not the form component itself) to show pending states
- **Return validation errors from Server Actions** as structured state, not thrown errors
- **Use `useOptimistic`** for any mutation where instant feedback matters (likes, adds, toggles)
- **Keep Server Actions thin**: validate -> mutate -> revalidate -> return result

### Audit Checklist

- [ ] Forms use `<form action={...}>` with Server Actions (not `onSubmit` + `fetch`)
- [ ] Submit buttons disable/show spinner via `useFormStatus`
- [ ] Mutations that benefit from instant feedback use `useOptimistic`
- [ ] Server Actions validate input and return structured error state
- [ ] Forms work without JavaScript (progressive enhancement)

---

## 3. Data Fetching Patterns

### Server-Side (Preferred)

- **Fetch in Server Components** directly — `async function Page()` with `await`
- **Parallel fetching**: Use `Promise.all` or start fetches before awaiting to avoid waterfalls
- **Sequential fetching**: Only when one fetch depends on another's result
- **`cache()` and `React.cache()`**: Deduplicate identical requests across the component tree
- **Server Actions for mutations only** — don't use POST-based Server Actions for reads (violates REST, breaks caching)

### Client-Side (When Needed)

- **TanStack Query / SWR** for client-side data that needs polling, optimistic updates, or real-time sync
- **`use()` hook** (React 19): Unwraps promises in render — use with Suspense for concurrent data loading
- **Avoid `useEffect` for initial data fetching** — prefer Server Components or Suspense + `use()`

### Audit Checklist

- [ ] Initial page data is fetched server-side (not in `useEffect`)
- [ ] Parallel fetches use `Promise.all` or concurrent patterns
- [ ] No waterfalls from sequential `await` chains where data is independent
- [ ] Client-side polling/real-time data uses TanStack Query or SWR
- [ ] Server Actions are used for writes, not reads

---

## 4. State Management

The modern approach separates **server state** from **client state**.

### Server State

- **TanStack Query** or **SWR** for caching, deduplication, background refetching, optimistic updates
- Or: rely on Next.js Server Components + revalidation (`revalidatePath` / `revalidateTag`)
- Offloading server state to these tools typically shrinks global client state by ~90%

### Client State

| Scope | Solution |
|-------|----------|
| Component-local | `useState`, `useReducer` |
| Shared across nearby components | Lift state up, or composition via props |
| Low-frequency global (theme, auth, locale) | React Context |
| High-frequency global (UI state, complex forms) | Zustand (minimal API, fine-grained subscriptions) |

### Anti-Patterns

- Don't put server/fetched data in Redux/Zustand — use a server-state library
- Don't stack 5+ Context providers — consider Zustand or component composition instead
- Don't sync server state into client state (`useEffect` + `setState` from fetch result)
- Don't use global state for data that only one component tree needs

### Audit Checklist

- [ ] Server data is managed via Server Components or a server-state lib (not Redux/Zustand)
- [ ] Context is used sparingly for low-frequency globals only
- [ ] No `useEffect` + `setState` patterns for syncing server data into client state
- [ ] Complex client state uses `useReducer` or Zustand, not nested `useState`

---

## 5. React Compiler & Memoization

React Compiler 1.0 is **stable and shipping at Meta**. It automatically memoizes components and hooks at build time. In Next.js 16, enable it via `reactCompiler: true` in `next.config.ts` (top-level, no longer under `experimental`).

### What Changes

- `React.memo()`, `useMemo()`, `useCallback()` are **no longer necessary for most cases**
- The compiler applies optimal memoization automatically — fewer re-renders without manual work
- Real-world results: up to 12% faster loads, 2.5x quicker interactions
- `eslint-plugin-react-hooks` v6 includes React Compiler-powered lint rules

### When to Still Use Manual Memoization

- Third-party libs that compare by reference identity
- Effect dependencies where semantic equality differs from referential equality
- Performance-critical paths where you need guaranteed caching behavior

### Migration Path

1. Enable React Compiler in `next.config.ts`: `reactCompiler: true`
2. Gradually remove `useMemo`/`useCallback`/`memo` wrappers
3. Keep them only where the compiler can't infer intent (rare)

### Audit Checklist

- [ ] React Compiler enabled in `next.config.ts`
- [ ] Identify `useMemo`/`useCallback`/`memo` usage — mark which are still needed
- [ ] No premature memoization in simple components
- [ ] Heavy computations in Server Components (no memoization needed server-side)
- [ ] `eslint-plugin-react-hooks` v6+ installed for Compiler-aware lint rules

---

## 6. Error Handling

### Error Boundaries

- Catch rendering errors in a subtree, display fallback UI
- **Don't catch**: event handlers, async code (outside render), SSR errors, errors in the boundary itself
- Use `error.tsx` (Next.js convention) for route-level error boundaries
- Use custom `<ErrorBoundary>` for component-level granularity

### Suspense + Error Boundary Pattern

```
<ErrorBoundary fallback={<ErrorFallback />}>
  <Suspense fallback={<Skeleton />}>
    <AsyncComponent />
  </Suspense>
</ErrorBoundary>
```

- Suspense handles loading -> Error Boundary catches failures
- With streaming SSR, if a component throws on the server, React renders the nearest Suspense fallback in the HTML

### `use()` Hook Error Handling

- When a promise passed to `use()` rejects, the error propagates to the nearest Error Boundary
- Combine with Suspense for the complete loading -> success -> error flow

### Best Practices

- Place Error Boundaries at **meaningful UI boundaries** (page, panel, widget) — not every component
- Provide **recovery actions** in error fallbacks (retry button, navigation link)
- Log errors to an external service (Sentry, etc.) from the error boundary
- Use `notFound()` for 404s in Server Components — renders `not-found.tsx`

### Audit Checklist

- [ ] Route-level `error.tsx` files exist for critical pages
- [ ] Error boundaries provide recovery actions, not just "Something went wrong"
- [ ] Suspense + Error Boundary pattern used for async data
- [ ] Errors are logged to an external service
- [ ] `notFound()` used for missing resources (not manual 404 rendering)

---

## 7. Component Architecture

### Composition Over Props

- **Compound Components**: Parent + children share implicit state via Context (e.g., `<Tabs>` + `<Tab>`)
- **Children as slots**: Pass components as children/props instead of config objects
- Avoid "prop soup" — if a component has 10+ props, it probably needs composition

### Layered Architecture

1. **Primitives** (headless): Unstyled, accessible base components (Headless UI, Radix, React Aria)
2. **Design system**: Styled primitives themed to your brand
3. **Domain components**: Business-specific compositions of design system components

### Import Rules

- Domain components -> may import design system + primitives
- Design system -> may import primitives only
- Primitives -> no project imports
- **No upward imports** (primitive importing a domain component)

### Best Practices

- **Single Responsibility**: Each component does one thing
- **Colocation**: Keep component + styles + tests + stories together
- **Explicit props over implicit behavior**: Prefer `<Button variant="danger">` over `<DangerButton>`
- **Avoid premature abstraction**: Three similar blocks are better than one over-engineered generic component
- **Children pattern for flexibility**: `<Card>{children}</Card>` > `<Card title="..." body="..." />`

### Audit Checklist

- [ ] Component layers have clear import boundaries (no upward imports)
- [ ] Complex components use compound/composition pattern (not mega-props)
- [ ] Components, stories, and tests are colocated
- [ ] No premature abstractions (components used in only one place shouldn't be in a shared folder)
- [ ] Headless UI primitives used for accessibility-critical components

---

## 8. TypeScript Patterns

### Strict Configuration

```json
{
  "strict": true,
  "noUncheckedIndexedAccess": true,
  "noImplicitOverride": true,
  "exactOptionalPropertyTypes": true
}
```

### Component Props

- **Explicit interfaces** over inline types: `interface ButtonProps { ... }` not `(props: { ... })`
- **Discriminated unions** for variant-dependent props:
  ```ts
  type AlertProps =
    | { variant: "info"; icon?: ReactNode }
    | { variant: "error"; onRetry: () => void }
  ```
- **`ComponentPropsWithoutRef<"button">`** to extend native HTML element props
- **Never use `any`** — use `unknown` and narrow with type guards

### Hooks

- **Generics for reusable hooks**: `function useLocalStorage<T>(key: string, initial: T): [T, (v: T) => void]`
- **Return tuples** (`as const`) for hooks that return `[value, setter]`
- **Discriminated union return types** for async hooks:
  ```ts
  type UseQueryResult<T> =
    | { status: "loading" }
    | { status: "error"; error: Error }
    | { status: "success"; data: T }
  ```

### Type Guards

- Create custom type guards for runtime narrowing: `function isUser(v: unknown): v is User`
- Prefer exhaustive `switch` with `never` default for discriminated unions

### Audit Checklist

- [ ] `strict: true` enabled in tsconfig
- [ ] No `any` types (use `unknown` + type guards)
- [ ] Component props use explicit interfaces
- [ ] Variant-dependent props use discriminated unions
- [ ] Hook return types use tuples or discriminated unions
- [ ] `noUncheckedIndexedAccess` enabled for safer array/object access

---

## 9. Performance

### Bundle Size

- **Server Components** eliminate JS for non-interactive parts (biggest win)
- **Dynamic imports**: `React.lazy()` or `next/dynamic` for heavy components
- **Tree shaking**: Use named exports; avoid barrel files (`index.ts` re-exporting everything)
- **Analyze bundles**: `@next/bundle-analyzer` to find bloat

### Rendering

- **Avoid unnecessary re-renders**: React Compiler handles most cases; for the rest, check component boundaries
- **Virtualize long lists**: `react-window` or `@tanstack/virtual` for 100+ items
- **Debounce expensive operations**: Search inputs, resize handlers
- **`useTransition`** for non-urgent updates: Keep UI responsive during heavy state transitions

### Images & Media

- **`next/image`**: Automatic optimization, lazy loading, responsive sizes
- **Explicit `width`/`height`** or `fill` to prevent layout shift (CLS)

### Web Vitals

- **LCP**: Preload critical images/fonts, minimize server response time
- **INP**: Keep event handlers fast, use `useTransition` for heavy updates
- **CLS**: Set explicit dimensions on images/embeds, avoid layout-shifting content insertion

### Audit Checklist

- [ ] Server Components used for static/data-display parts
- [ ] Dynamic imports for heavy client components (modals, editors, charts)
- [ ] No barrel files re-exporting everything
- [ ] Long lists are virtualized
- [ ] Images use `next/image` with explicit dimensions
- [ ] `useTransition` used for non-urgent state updates (filtering, searching)

---

## 10. Accessibility

### Fundamentals

- **Semantic HTML first**: Use `<button>`, `<nav>`, `<main>`, `<article>` — not `<div onClick>`
- **Keyboard navigation**: All interactive elements reachable via Tab; custom widgets implement arrow-key navigation
- **Touch targets**: Minimum 44x44px for mobile
- **Color contrast**: WCAG AA minimum (4.5:1 for normal text, 3:1 for large text)

### ARIA

- **ARIA is a last resort** — semantic HTML is always preferred
- **Keep ARIA state in sync with React state**: If a dropdown is open, `aria-expanded` must be `true`
- **Live regions** (`aria-live="polite"`) for dynamic content changes (toast notifications, form errors)
- **Label all inputs**: `<label htmlFor>` or `aria-label` / `aria-labelledby`
- **`sr-only` class** for screen-reader-only text

### Focus Management

- **After client-side navigation**: Programmatically focus the new page's heading or `<main>`
- **Modals**: Trap focus inside, return focus to trigger on close
- **Skip links**: "Skip to main content" link as first focusable element

### Testing

- **Automated**: `axe-core` (via Storybook a11y addon or `jest-axe`) catches ~57% of issues
- **Manual**: Test with screen reader (VoiceOver on macOS) and keyboard-only navigation
- **Storybook**: A11y addon for per-story automated checks

### Regulatory Context

ADA Title II enforcement hits April 2026; European Accessibility Act is already in effect. Accessibility is a legal requirement, not optional.

### Audit Checklist

- [ ] No `<div onClick>` — interactive elements use `<button>`, `<a>`, etc.
- [ ] All inputs have associated labels
- [ ] Touch targets are 44x44px minimum
- [ ] Modals trap focus and restore it on close
- [ ] Dynamic content uses `aria-live` regions
- [ ] Storybook a11y addon is active and stories pass
- [ ] Color contrast meets WCAG AA

---

## 11. Testing Strategy

### Modern Testing Pyramid (2026)

1. **Unit tests** (Vitest 4): Pure functions, utilities, hooks — fast, isolated
2. **Component tests** (Storybook 10 interaction tests): Visual + behavioral tests in the browser — play functions simulate user interaction
3. **Integration tests**: Database operations, API routes — test real behavior with test DB
4. **E2E tests** (Playwright): Critical user flows end-to-end — slowest, most confidence

### Storybook 10 Interaction Tests

- **ESM-only** — no more CommonJS (requires Node 20.16+)
- Use `play` functions with `@storybook/test` for interaction tests
- **`sb.mock`** for module automocking — works with both Vite and Webpack, compatible with Vitest API
- **CSF Factories** (preview): New Component Story Format with better TypeScript support and less boilerplate
- Tests run in the browser — same environment as development
- Combine with a11y addon for accessibility testing per story

### Vitest 4

- **Browser mode is stable** (no longer experimental) — requires `@vitest/browser-playwright`
- **Visual regression testing**: Built-in `toMatchScreenshot` assertion
- **New matchers**: `expect.schemaMatching()` validates against Zod 4 / Standard Schema v1
- Context imports changed: `vitest/browser` (was `@vitest/browser/context`)

### Best Practices

- **Test behavior, not implementation**: Click buttons, fill forms, assert visible output — not internal state
- **Colocate tests**: `Component.tsx` + `Component.stories.tsx` + `Component.test.ts` in same directory
- **Mock at boundaries**: Use `sb.mock` in Storybook, mock API calls and external services — not internal modules
- **Avoid snapshot tests**: They break on any change and teach nothing about correctness
- **Use `testing-library` queries by role/label**: `getByRole("button", { name: "Submit" })` not `getById`

### Audit Checklist

- [ ] Unit tests for pure business logic (utils, validation, data transforms)
- [ ] Storybook stories exist for all UI components with play functions for interactive ones
- [ ] Storybook a11y addon active for automated accessibility checks
- [ ] E2E tests cover critical user flows (login, CRUD operations)
- [ ] Tests query by role/label, not by test-id or class
- [ ] No snapshot tests (or very few, well-justified ones)
- [ ] Tests colocated with source files
- [ ] Vitest browser mode used for component tests requiring real DOM

---

## 12. Security

### Server Actions

- **Validate all inputs** on the server — client validation is UX, not security
- **Use `zod` or similar** for runtime schema validation
- **Check authorization** in every Server Action — don't trust the client
- **Rate limit** sensitive actions (login, password reset)

### Authentication

- **HttpOnly cookies** for session tokens — never expose in JS
- **CSRF protection**: Forms with Server Actions get it for free (same-origin POST)
- **Validate `returnTo`/redirect URLs**: Must start with `/`, block `//` and `:` to prevent open redirects

### Data Exposure

- **Never pass sensitive data as props** from Server to Client Components — it's serialized to HTML
- **Use `server-only` package** to mark modules that must never be imported in Client Components
- **Environment variables**: Only `NEXT_PUBLIC_*` variables are exposed to the client

### Audit Checklist

- [ ] All Server Actions validate input with `zod` or equivalent
- [ ] Authorization checked in every Server Action (not just in middleware)
- [ ] Session tokens in HttpOnly cookies only
- [ ] Redirect URLs validated (no open redirects)
- [ ] No secrets/sensitive data passed as props to Client Components
- [ ] `server-only` package used for server-only modules

---

## 13. Project Structure & Conventions

### File Organization

```
app/
  (routes)/        # Route groups
    page.tsx       # Server Component by default
    loading.tsx    # Suspense fallback
    error.tsx      # Error boundary
    layout.tsx     # Shared layout
  lib/             # Shared utilities, business logic
  api/             # API routes (when needed)
components/
  ui/              # Primitives (Headless UI + custom)
  domain/          # Domain-specific components
db/
  migrations/      # SQL migration files
  repositories/    # Data access layer
```

### Naming Conventions

- **Files**: kebab-case (`user-profile.tsx`) or PascalCase for components (`UserProfile.tsx`)
- **Components**: PascalCase (`UserProfile`)
- **Hooks**: camelCase with `use` prefix (`useAuth`)
- **Server Actions**: camelCase with verb prefix (`createMatch`, `updateStandings`)
- **Types**: PascalCase, no `I` prefix (`UserProfile`, not `IUserProfile`)

### Imports

- **Absolute imports** via `@/` alias (configured in `tsconfig.json`)
- **No circular imports** — enforce with ESLint rules
- **Group imports**: React/Next -> third-party -> project modules -> relative

### Audit Checklist

- [ ] Route-level `loading.tsx` and `error.tsx` files exist
- [ ] Consistent file naming across the project
- [ ] Absolute imports used consistently
- [ ] No circular dependencies
- [ ] Server Actions separated from components (in `lib/` or `actions/`)
- [ ] Database access in a repository/data-access layer (not inline in components)

---

## Sources

- [React v19 Official Blog](https://react.dev/blog/2024/12/05/react-19)
- [React 19.2 Blog Post](https://react.dev/blog/2025/10/01/react-19-2)
- [Next.js Server & Client Components](https://nextjs.org/docs/app/getting-started/server-and-client-components)
- [Next.js Data Fetching Patterns](https://nextjs.org/docs/app/building-your-application/data-fetching/patterns)
- [React Compiler Introduction](https://react.dev/learn/react-compiler/introduction)
- [Meta's React Compiler 1.0 — InfoQ](https://www.infoq.com/news/2025/12/react-compiler-meta/)
- [React State Management in 2025](https://www.developerway.com/posts/react-state-management-2025)
- [React & Next.js in 2025 — Modern Best Practices](https://strapi.io/blog/react-and-nextjs-in-2025-modern-best-practices)
- [React Server Components Practical Guide 2026](https://inhaq.com/blog/react-server-components-practical-guide-2026)
- [Storybook 10 Blog Post](https://storybook.js.org/blog/storybook-10/)
- [Storybook Interaction Testing](https://storybook.js.org/docs/writing-tests/interaction-testing)
- [Vitest 4.0 Blog Post](https://vitest.dev/blog/vitest-4)
- [Next.js Forms with Server Actions](https://www.robinwieruch.de/next-forms/)
- [React Accessibility — React Aria](https://react-spectrum.adobe.com/react-aria/accessibility.html)
- [Advanced TypeScript for React — Discriminated Unions](https://www.developerway.com/posts/advanced-typescript-for-react-developers-discriminated-unions)
- [State Management Trends 2025 — Makers' Den](https://makersden.io/blog/react-state-management-in-2025)
- [Compound Components — patterns.dev](https://www.patterns.dev/react/compound-pattern/)
