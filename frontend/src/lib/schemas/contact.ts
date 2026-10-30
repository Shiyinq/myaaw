import { z } from 'zod';
import { nameSchema, optionalTextSchema } from './primitives';

export const contactSchema = z.object({
	name: nameSchema,
	url: z.url('url'),
	token: z.string().trim().optional(),
	description: optionalTextSchema.optional()
});

export type ContactForm = z.infer<typeof contactSchema>;
