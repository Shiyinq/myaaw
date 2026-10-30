import { z } from 'zod';
import { nameSchema, optionalTextSchema, optionalUrlSchema } from './primitives';

export const onboardSchema = z.object({
	name: nameSchema,
	provider_type: z.enum(['openai', 'gemini']),
	base_url: optionalUrlSchema,
	api_key: z.string().optional(),
	model: optionalTextSchema.optional()
});

export type OnboardForm = z.infer<typeof onboardSchema>;
