import { z } from 'zod';
import { nameSchema, optionalTextSchema, optionalUrlSchema } from './primitives';

export const providerSchema = z.object({
	name: nameSchema,
	type: z.enum(['openai', 'gemini']),
	base_url: optionalUrlSchema,
	default_model: optionalTextSchema.optional(),
	api_key: z.string().optional()
});

export type ProviderForm = z.infer<typeof providerSchema>;

export const modelLookupSchema = z.object({
	type: z.enum(['openai', 'gemini']),
	base_url: optionalUrlSchema,
	api_key: z.string().optional()
});
