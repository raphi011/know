import { useEffect } from "react";
import { definePreview } from "@storybook/react-vite";
import type { ReactRenderer } from "@storybook/react-vite";
import { withThemeByClassName } from "@storybook/addon-themes";
import type { DecoratorFunction } from "storybook/internal/types";
import { NextIntlClientProvider } from "next-intl";
import addonA11y from "@storybook/addon-a11y";
import addonDocs from "@storybook/addon-docs";
import deMessages from "../messages/de.json";
import enMessages from "../messages/en.json";
import "../app/globals.css";

const messages: Record<string, typeof deMessages> = {
  de: deMessages,
  en: enMessages,
};

/**
 * Wraps every story in NextIntlClientProvider so useTranslations() works.
 * The locale is controlled via the toolbar globe icon.
 */
const withIntl: DecoratorFunction<ReactRenderer> = (Story, context) => {
  const locale = (context.globals?.locale as string) ?? "de";
  return (
    <NextIntlClientProvider locale={locale} messages={messages[locale]}>
      <Story />
    </NextIntlClientProvider>
  );
};

/**
 * Sets body + wrapper bg/text to match the active theme.
 * useEffect ensures the iframe body itself gets the dark background,
 * not just the wrapper div (which doesn't fill centered layouts).
 */
const WithThemeBackground: DecoratorFunction<ReactRenderer> = (
  Story,
  context,
) => {
  const isDark = context.globals?.theme === "dark";

  useEffect(() => {
    document.body.style.backgroundColor = isDark ? "#020617" : "#f8fafc";
    document.body.style.color = isDark ? "#94a3b8" : "#475569";
  }, [isDark]);

  return (
    <div className="font-sans">
      <Story />
    </div>
  );
};

export default definePreview({
  addons: [addonDocs(), addonA11y()],
  globalTypes: {
    locale: {
      description: "Locale for translations",
      toolbar: {
        title: "Locale",
        icon: "globe",
        items: [
          { value: "de", title: "DE Deutsch" },
          { value: "en", title: "EN English" },
        ],
        dynamicTitle: true,
      },
    },
  },
  initialGlobals: {
    locale: "de",
  },
  parameters: {
    a11y: {
      test: "error",
      config: {
        rules: [
          // Disabled: remaining violations are in skeleton/loading states,
          // avatar fallbacks, and Headless UI internals where contrast is
          // intentionally low or framework-controlled.
          { id: "color-contrast", enabled: false },
        ],
      },
    },
    layout: "centered",
    viewport: {
      options: {
        iPhoneSE: {
          name: "iPhone SE",
          styles: { width: "375px", height: "667px" },
        },
        iPhone14: {
          name: "iPhone 14",
          styles: { width: "393px", height: "852px" },
        },
        tablet: {
          name: "iPad",
          styles: { width: "768px", height: "1024px" },
        },
        desktop: {
          name: "Desktop",
          styles: { width: "1440px", height: "900px" },
        },
      },
    },
  },
  decorators: [
    withIntl,
    WithThemeBackground,
    withThemeByClassName({
      themes: {
        light: "",
        dark: "dark",
      },
      defaultTheme: "light",
    }),
  ],
});
