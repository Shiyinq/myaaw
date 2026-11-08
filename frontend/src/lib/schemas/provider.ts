import { z } from 'zod';
import { nameSchema, optionalTextSchema, optionalUrlSchema } from './primitives';

export const providerSchema = z.object({
	name: nameSchema,
	type: z.enum(['openai', 'gemini', 'openai_chatgpt']),
	base_url: optionalUrlSchema.optional(),
	default_model: optionalTextSchema.optional(),
	api_key: z.string().optional(),
	auth_mode: z.literal('oauth').optional(),
	credential_id: z.string().trim().min(1).optional()
});

export type ProviderForm = z.infer<typeof providerSchema>;

export const modelLookupSchema = z.object({
	type: z.enum(['openai', 'gemini', 'openai_chatgpt']),
	base_url: optionalUrlSchema.optional(),
	api_key: z.string().optional(),
	credential_id: z.string().trim().min(1).optional()
});
