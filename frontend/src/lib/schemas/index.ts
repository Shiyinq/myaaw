import { z } from 'zod';

// Auth Schemas
export const loginSchema = z.object({
	username: z.string().min(3, 'Username must be at least 3 characters'),
	password: z.string().min(6, 'Password must be at least 6 characters')
});

export const registerSchema = z
	.object({
		name: z.string().min(2, 'Name is too short'),
		username: z.string().min(3, 'Username is too short'),
		email: z.string().email('Invalid email address'),
		password: z.string().min(8, 'Password must be at least 8 characters'),
		confirmPassword: z.string()
	})
	.refine((data) => data.password === data.confirmPassword, {
		message: "Passwords don't match",
		path: ['confirmPassword']
	});

// API Key Schemas
export const apiKeyResponseSchema = z.object({
	apiKey: z.string()
});

export const userProfileSchema = z.object({
	id: z.number().optional(),
	userId: z.string(),
	name: z.string(),
	username: z.string(),
	email: z.string().email()
});

// Types inferred from schemas
export type LoginData = z.infer<typeof loginSchema>;
export type RegisterData = z.infer<typeof registerSchema>;
export type ApiKeyResponse = z.infer<typeof apiKeyResponseSchema>;
export type UserProfile = z.infer<typeof userProfileSchema>;
