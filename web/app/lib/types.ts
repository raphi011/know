export const LOCALES = ["de", "en"] as const;
export type Locale = (typeof LOCALES)[number];

export const THEMES = ["light", "dark", "auto"] as const;
export type Theme = (typeof THEMES)[number];
