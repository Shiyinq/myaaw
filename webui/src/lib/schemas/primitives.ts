import { z } from 'zod';

/** `myaaw.validation.*` message codes shared by every form schema. */
export const required = 'required';
export const tooLong = 'too_long';
export const invalidIdentifier = 'identifier';
export const invalidUrl = 'url';
export const invalidJson = 'json';
export const invalidCron = 'cron';
export const invalidEnv = 'env';

const identifier = /^[a-zA-Z0-9._-]+$/;

export const nameSchema = z
	.string()
	.trim()
	.min(1, required)
	.max(64, tooLong)
	.regex(identifier, invalidIdentifier);

/** A single path segment: file or folder name without separators. */
export const fileNameSchema = z
	.string()
	.trim()
	.min(1, required)
	.max(120, tooLong)
	.regex(/^[^/\\]+$/, 'no_slashes');

/** Optional http(s) URL that also accepts an empty string from a text input. */
export const optionalUrlSchema = z.union([z.literal(''), z.url(invalidUrl)]);

export const optionalTextSchema = z.string().trim().max(512, tooLong);

export function isValidJson(value: string) {
	if (value.trim() === '') return true;
	try {
		JSON.parse(value);
		return true;
	} catch {
		return false;
	}
}

export function isValidEnv(value: string) {
	return value
		.split('\n')
		.map((line) => line.trim())
		.filter(Boolean)
		.every((line) => {
			const index = line.indexOf('=');
			return index > 0 && line.slice(0, index).trim() !== '';
		});
}

/** Five-field cron, `@hourly`-style macros are not accepted. */
export function isValidCron(value: string) {
	return value.trim().split(/\s+/).length === 5;
}
