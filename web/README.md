# Webapp Template

A Next.js 16 webapp template with Auth.js (OIDC), Drizzle ORM, Storybook, and i18n.

## Getting Started

### Prerequisites

- [Bun](https://bun.sh/) (v1.0+)
- [Docker](https://www.docker.com/) (for PostgreSQL)

### Setup

```bash
# 1. Install dependencies
bun install

# 2. Start PostgreSQL
docker-compose up -d

# 3. Copy environment variables
cp .env.local.example .env.local
# Edit .env.local with your OIDC provider settings

# 4. Run database migrations
bun run db:migrate

# 5. Seed example data
bun run db:seed

# 6. Start dev server
bun run dev
```

Open [http://localhost:3000](http://localhost:3000).

### Storybook

```bash
bun storybook
```

Open [http://localhost:6006](http://localhost:6006).

## Project Structure

```
app/
├── (auth)/          # Public auth pages
├── (main)/          # Authenticated pages
├── api/             # API routes
├── lib/
│   ├── actions/     # Server actions
│   ├── db/          # Drizzle schema & queries
│   ├── auth.ts      # Auth.js config
│   └── env.ts       # Env var access
components/
├── ui/              # Primitives (Button, Input, etc.)
└── *.tsx            # Composites (Card, AppShell, etc.)
stories/             # Storybook stories
docs/                # Project documentation
e2e/                 # Playwright E2E tests
db/
├── migrations/      # Drizzle migrations
└── seed.ts          # Seed script
```

## Scripts

| Command | Description |
|---------|-------------|
| `bun run dev` | Start dev server |
| `bun storybook` | Start Storybook |
| `bun run lint` | Run ESLint |
| `bun run typecheck` | Type check |
| `bun run test:ci` | Run Storybook tests |
| `bun run test:e2e` | Run E2E tests |
| `bun run db:migrate` | Run migrations |
| `bun run db:seed` | Seed database |
