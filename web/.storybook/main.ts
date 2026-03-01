import { fileURLToPath } from "node:url";
import path, { dirname } from "node:path";
import type { StorybookConfig } from "@storybook/react-vite";
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import remarkGfm from "remark-gfm";

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

const config: StorybookConfig = {
  stories: [
    "../docs/**/*.mdx",
    "../stories/**/*.stories.@(ts|tsx)",
    "../components/**/*.stories.@(ts|tsx)",
  ],
  addons: [
    "@storybook/addon-themes",
    {
      name: "@storybook/addon-docs",
      options: {
        mdxPluginOptions: {
          mdxCompileOptions: {
            remarkPlugins: [remarkGfm],
          },
        },
      },
    },
    "@storybook/addon-a11y",
    "@storybook/addon-vitest",
  ],
  framework: {
    name: "@storybook/react-vite",
    options: {},
  },
  viteFinal(config) {
    config.resolve = config.resolve || {};
    config.resolve.alias = {
      ...config.resolve.alias,
      "next/cache": path.resolve(__dirname, "mocks/next-cache.ts"),
      "next/headers": path.resolve(__dirname, "mocks/next-headers.ts"),
      "next/link": path.resolve(__dirname, "mocks/next-link.tsx"),
      "next/navigation": path.resolve(__dirname, "mocks/next-navigation.ts"),
      "@/app/lib/actions/theme": path.resolve(
        __dirname,
        "mocks/theme-action.ts",
      ),
      "server-only": path.resolve(__dirname, "mocks/server-only.ts"),
      "@": path.resolve(__dirname, ".."),
    };
    config.plugins = config.plugins || [];
    config.plugins.push(tailwindcss());
    config.plugins.push(react());

    config.build = {
      ...config.build,
      chunkSizeWarningLimit: 1300,
      rollupOptions: {
        ...config.build?.rollupOptions,
        output: {
          ...(config.build?.rollupOptions?.output as object),
          manualChunks(id: string) {
            if (id.includes("node_modules/react-dom")) return "react-vendor";
            if (id.includes("node_modules/react/")) return "react-vendor";
            if (id.includes("node_modules/scheduler")) return "react-vendor";
            if (id.includes("node_modules/@storybook/"))
              return "storybook-vendor";
            if (id.includes("node_modules/@headlessui/")) return "headlessui";
            if (
              id.includes("node_modules/next-intl") ||
              id.includes("node_modules/intl-messageformat")
            )
              return "i18n";
          },
        },
      },
    };

    return config;
  },
};

export default config;
