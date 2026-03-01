/** Discriminated union for server action outcomes. Use `result.success` to narrow. */
export type ActionResult =
  | { success: true }
  | { success: false; error: string };

/** Extended variant when the success case carries extra data. */
export type ActionResultWith<T> =
  | ({ success: true } & T)
  | { success: false; error: string };

export function isActionError(
  result: ActionResult | ActionResultWith<unknown>,
): result is { success: false; error: string } {
  return !result.success;
}
