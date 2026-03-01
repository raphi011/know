# Testing Strategy

Three-layer testing approach: **Unit tests** for pure domain logic, **Storybook interaction tests** for UI behavior, and **Playwright e2e tests** for full-stack flows.

---

## Unit Tests (Domain Logic)

Pure functions that encode business rules — no DB, no server, no React. These are the fastest tests in the suite.

**What they test:**
- Validation rules (input sanitization, format checks)
- Data transformation logic (mapping, filtering, aggregation)
- Permission checks (role-based access helpers)
- Date/time utilities
- Slug/ID generation and validation

**How they work:**
- Plain Vitest tests in Node.js (same "db" project config, but no DB needed)
- Tests live alongside the modules they test (e.g. `app/lib/validation.test.ts`)
- **Table-driven tests** for combinatorial logic — define inputs and expected outputs as arrays of cases

**Commands:**

```bash
bun run test:ci               # Runs unit + Storybook tests
```

**Example (table-driven):**

```ts
// app/lib/__tests__/validation.test.ts
import { isValidSlug } from "../validation";

describe("isValidSlug", () => {
  const cases = [
    { input: "hello-world", expected: true, reason: "valid kebab-case" },
    { input: "Hello World", expected: false, reason: "spaces not allowed" },
    { input: "", expected: false, reason: "empty string" },
    { input: "a".repeat(256), expected: false, reason: "too long" },
    { input: "hello_world", expected: true, reason: "underscores allowed" },
  ];

  it.each(cases)(
    "$input → $expected ($reason)",
    ({ input, expected }) => {
      expect(isValidSlug(input)).toBe(expected);
    },
  );
});
```

---

## Storybook Interaction Tests

Component-level tests that run inside Storybook via `@storybook/addon-vitest` + Playwright browser mode.

**What they test:**
- UI rendering, form validation, state changes
- User interactions (click, type, select, drag)
- Accessibility (axe-core via `@storybook/addon-a11y`)
- Responsive behavior (viewport parameters)
- Loading, empty, and error states

**How they work:**
- Stories import real components from `app/` and `components/`
- `play` functions define interaction sequences and assertions
- Run in a real browser (Chromium via `@vitest/browser-playwright`)
- No server, database, or API needed — data passed as props

**Commands:**

```bash
bun run test              # Watch mode
bun run test:ci           # Single run (CI)
bun run test:coverage     # With Istanbul coverage
```

**Example:**

```tsx
export const SubmitForm = meta.story({
  render: () => <FormSheet onSubmit={mockSubmit} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const button = canvas.getByRole("button", { name: /submit/i });

    // Button disabled by default (required fields empty)
    await expect(button).toBeDisabled();

    // Fill required fields
    const titleInput = canvas.getByLabelText("Title");
    await userEvent.type(titleInput, "My Post Title");
    await expect(button).toBeEnabled();
  },
});
```

---

## Playwright E2E Tests

Full-stack tests against a running Next.js server with a real database.

**What they test:**
- Auth flows (login → session cookie → redirect)
- API routes and server actions
- Middleware behavior (route protection, session expiry)
- Multi-step user flows

**How they work:**
- Playwright drives a real Chromium browser against `localhost:3000`
- Tests live in `e2e/` directory, named `*.spec.ts`
- `playwright.config.ts` auto-starts the dev server
- Two projects: desktop Chrome + mobile viewport (390x844)
- No database required — the web frontend is fully stateless (encrypted cookies)

**Commands:**

```bash
bun run test:e2e          # Headless run
bun run test:e2e:ui       # Interactive UI mode
```

**Example:**

```ts
test("unauthenticated user is redirected to /login", async ({ page }) => {
  await page.goto("/");
  await expect(page).toHaveURL(/\/login/);
});
```

---

## When to Use Which Layer

| Scenario | Layer |
|----------|-------|
| Input validation rules | Unit (table-driven) |
| Data transformation / formatting | Unit (table-driven) |
| Permission check logic | Unit (table-driven) |
| Form validation (empty fields, invalid input) | Storybook |
| Button states (disabled, loading) | Storybook |
| Dialog/sheet open and close | Storybook |
| Card layout and indicators | Storybook |
| Component accessibility | Storybook |
| Login → session cookie → redirect | Playwright |
| Settings change → persistence | Playwright |
| Middleware redirects (expired session) | Playwright |
| useActionState form submission flow | Storybook |
| Suspense fallback rendering | Storybook |
| Progressive enhancement (form works without JS) | Playwright |

**Rule of thumb:** Pure business logic with no I/O → unit test (table-driven). UI behavior with mock data → Storybook. Full-stack flow with server → Playwright.

---

## File Structure

```
├── app/lib/                     # Application logic + colocated tests
│   ├── validation.ts           # Input validation rules
│   ├── validation.test.ts      # Unit tests for validation
│   ├── formatting.ts           # Data formatting utils
│   ├── formatting.test.ts      # Unit tests for formatting
│   ├── session.ts             # Encrypted cookie session (AES-256-GCM)
│   ├── actions/auth.ts        # Login/logout server actions
│   └── ...
├── e2e/                        # Playwright e2e tests
│   ├── smoke.spec.ts           # Basic app reachability
│   ├── auth.spec.ts            # Login, session cookie, logout
│   └── ...
├── stories/                    # Storybook stories + interaction tests
│   ├── ui/                     # Layer 1: Headless UI + custom primitives
│   ├── composites/             # Layer 2: Composite components
│   ├── domain/                 # Layer 3: Domain components
│   └── pages/                  # Full page compositions
├── playwright.config.ts        # Playwright configuration
├── vitest.config.ts            # Vitest + Storybook + DB test config
└── .storybook/
    ├── main.ts                 # Storybook config
    ├── preview.tsx             # Decorators, globals, a11y
    └── vitest.setup.ts         # Vitest setup for Storybook
```

---

## Conventions

### Story Files

- Stories import **real components**, not inline definitions
- Every interactive story has a `play` function with assertions
- Use `within(canvasElement)` for scoped queries
- Prefer accessible queries: `getByRole`, `getByLabelText`, `getByText`
- Test both happy path and edge cases (empty state, error state) as separate stories

### E2E Test Files

- One file per domain: `auth.spec.ts`, `posts.spec.ts`, `settings.spec.ts`
- Use `test.describe()` to group related flows
- Use page objects or helper functions for repeated interactions (login, seed data)
- Keep tests independent — each test can run in isolation
- Use `test.slow()` for multi-step flows that need extra time

### Naming

```
# Stories
stories/pages/Login.stories.tsx        → "Pages/Login"
stories/domain/PostCard.stories.tsx    → "Domain/PostCard"

# E2E tests
e2e/auth.spec.ts                       → "auth > login flow"
e2e/posts.spec.ts                      → "posts > create post"
```

---

## Implementation Order

User stories are implemented in dependency order. Each story gets both test layers where applicable.

| Phase | User Stories | Storybook | Playwright |
|-------|-------------|-----------|------------|
| 1. Auth | AUTH-01→05 | Login form, server connection | Login flow, session cookie, redirect |
| 2. Documents | DOC-01→04 | Document editor, sidebar, search | Browse, edit, save documents |
| 3. Chat | CHAT-01→03 | Chat panel, message list | Multi-turn Q&A, streaming |
| 4. Settings | SETT-01→03 | Settings page, theme toggle, language select | Update settings → persistence |
| 5. Polish | Remaining P1/P2 | Per-story | Per-story |

---

## Environment Setup

The web frontend is fully stateless — no database needed. Authentication uses encrypted httpOnly cookies (AES-256-GCM) storing the Knowhow server URL and API token.

### Local Development

```bash
just web-dev                  # Start Next.js dev server
# Open http://localhost:3000, log in with your Knowhow server URL + API token
```

Set in `.env`:
```
SESSION_SECRET=your-secret-here
```

### Test Isolation

- **E2E tests:** Each test is independent, no shared state needed
- **Storybook tests:** Components receive data via props, no server required

---

## CI Integration

Current CI pipeline (`.github/workflows/ci.yml`):

```
1. bun install
2. bun run typecheck               # TypeScript checker
3. bun run lint                    # ESLint
4. bun run test:ci                 # Storybook interaction tests
5. bun run test:e2e                # Playwright e2e (against dev server)
```

Steps 2-4 run in the lint/typecheck job. Step 5 runs in the e2e job. No database service container needed.
