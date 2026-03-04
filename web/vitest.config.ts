import path from "node:path";
import { fileURLToPath } from "node:url";

import { defineConfig } from "vitest/config";

import { storybookTest } from "@storybook/addon-vitest/vitest-plugin";

import { playwright } from "@vitest/browser-playwright";

const dirname = path.dirname(fileURLToPath(import.meta.url));

export default defineConfig({
  resolve: {
    alias: {
      "@": dirname,
    },
  },
  test: {
    coverage: {
      provider: "istanbul",
      include: ["components/**/*.tsx"],
      exclude: ["**/*.stories.tsx", "**/*.test.tsx", "**/*.test.ts"],
      reporter: ["text", "html", "json-summary"],
      reportsDirectory: "coverage",
    },
    projects: [
      {
        extends: true,
        plugins: [
          storybookTest({ configDir: path.join(dirname, ".storybook") }),
        ],
        optimizeDeps: {
          include: [
            "@headlessui/react",
            "@heroicons/react/24/outline",
            "@heroicons/react/20/solid",
            "clsx",
            "framer-motion",
            "storybook/test",
          ],
        },
        test: {
          name: "storybook",
          browser: {
            enabled: true,
            headless: true,
            provider: playwright({}),
            instances: [{ browser: "chromium" }],
          },
          setupFiles: ["./.storybook/vitest.setup.ts"],
        },
      },
      {
        extends: true,
        test: {
          name: "unit",
          include: [
            "app/lib/**/*.test.ts",
            "lib/**/*.test.ts",
            "*.test.ts",
            "components/**/*.test.tsx",
          ],
          environment: "node",
        },
      },
    ],
  },
});
