"use server";

import { setLocaleCookie } from "@/app/lib/auth";
import { LOCALES } from "@/app/lib/types";
import type { Locale } from "@/app/lib/types";
import { revalidatePath } from "next/cache";
import type { ActionResult } from "@/app/lib/action-result";

export async function updateLanguageAction(
  locale: Locale,
): Promise<ActionResult> {
  if (!LOCALES.includes(locale)) {
    return { success: false, error: "Invalid locale" };
  }
  try {
    await setLocaleCookie(locale);
  } catch (error) {
    console.error("Failed to update locale", error);
    return { success: false, error: "Failed to update language" };
  }
  revalidatePath("/", "layout");
  return { success: true };
}
