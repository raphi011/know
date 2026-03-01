/**
 * Shared viewport parameters for stories that need a specific default viewport.
 *
 * Storybook 10 removed `defaultViewport`. Setting `viewport.options` at the
 * story level replaces the global options (shallow merge), so we duplicate
 * only the viewport we need. Import this constant to avoid copy-pasting.
 */
export const mobileViewport = {
  viewport: {
    options: {
      iPhoneSE: {
        name: "iPhone SE",
        styles: { width: "375px", height: "667px" },
      },
    },
  },
} as const;
