import { z } from 'zod';
import {
	invalidEnv,
	invalidJson,
	isValidEnv,
	isValidJson,
	nameSchema,
	required
} from './primitives';

export const customToolSchema = z.object({
	name: nameSchema,
	command: z.string().trim().min(1, required),
	args: z.string().trim().optional(),
	schema: z.string().refine(isValidJson, invalidJson)
});

export type CustomToolForm = z.infer<typeof customToolSchema>;

export const mcpServerSchema = z.object({
	name: nameSchema,
	command: z.string().trim().min(1, required),
	args: z.string().trim().optional(),
	env: z.string().refine(isValidEnv, invalidEnv)
});

export type MCPServerForm = z.infer<typeof mcpServerSchema>;
