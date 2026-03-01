"use server";

import { setThemeCookie } from "@/app/lib/auth";
import { THEMES } from "@/app/lib/types";
import type { Theme } from "@/app/lib/types";
import type { ActionResult } from "@/app/lib/action-result";

export async function updateThemeAction(theme: Theme): Promise<ActionResult> {
  if (!THEMES.includes(theme)) {
    return { success: false, error: "Invalid theme" };
  }
  try {
    await setThemeCookie(theme);
  } catch (error) {
    console.error("Failed to update theme", error);
    return { success: false, error: "Failed to update theme" };
  }
  return { success: true };
}
