export function revalidatePath() {}
export function revalidateTag() {}
export function unstable_cache<T>(fn: () => T) {
  return fn;
}
