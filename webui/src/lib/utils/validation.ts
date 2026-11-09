import type { ZodType } from 'zod';

export type FieldErrors = Record<string, string>;

export type ValidationResult<T> = { ok: true; data: T } | { ok: false; errors: FieldErrors };

/**
 * Runs a zod schema and flattens the issues into `{ field: code }`, keeping the
 * first message per field. Codes are short (`required`, `url`, ...) and are
 * translated by the caller through `myaaw.validation.<code>`.
 */
export function validate<T>(schema: ZodType<T>, value: unknown): ValidationResult<T> {
	const result = schema.safeParse(value);
	if (result.success) return { ok: true, data: result.data };

	const errors: FieldErrors = {};
	for (const issue of result.error.issues) {
		const field = issue.path.join('.') || '_';
		if (!errors[field]) errors[field] = issue.message;
	}
	return { ok: false, errors };
}

export function hasErrors(errors: FieldErrors) {
	return Object.keys(errors).length > 0;
}
