import type { ZodType } from "zod";

type ParseSuccess<T> = { success: true; data: T };
type ParseFailure = {
  success: false;
  error: string;
  fieldErrors: Record<string, string>;
};

export function parseFormData<T>(
  schema: ZodType<T>,
  formData: FormData,
): ParseSuccess<T> | ParseFailure {
  const raw = Object.fromEntries(formData.entries());
  const result = schema.safeParse(raw);

  if (result.success) {
    return { success: true, data: result.data };
  }

  const fieldErrors: Record<string, string> = {};
  for (const issue of result.error.issues) {
    const field = issue.path.join(".");
    if (field && !fieldErrors[field]) {
      fieldErrors[field] = issue.message;
    }
  }
  return {
    success: false,
    error: result.error.issues[0]?.message ?? "Invalid input",
    fieldErrors,
  };
}
