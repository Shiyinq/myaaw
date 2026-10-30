import { z } from 'zod';
import { nameSchema, optionalUrlSchema, required, tooLong } from './primitives';

/** Telegram bot tokens look like `<bot-id>:<secret>`. */
const telegramTokenSchema = z
	.string()
	.trim()
	.max(256, tooLong)
	.refine((value) => value === '' || value.includes(':'), 'telegram_token');

/** Telegram requires a public https base URL without the webhook path. */
const webhookUrlSchema = z
	.string()
	.trim()
	.max(512, tooLong)
	.refine(
		(value) =>
			value === '' || (value.startsWith('https://') && !value.endsWith('/webhook/telegram')),
		'webhook_url'
	);

/** Chat and owner IDs are numeric, optionally negative (Discord guilds). */
const chatIdSchema = z
	.string()
	.trim()
	.max(120, tooLong)
	.refine((value) => value === '' || /^-?\d+$/.test(value), 'chat_id');

const ownerIdsSchema = z
	.string()
	.trim()
	.max(512, tooLong)
	.refine(
		(value) =>
			value === '' ||
			value
				.split(',')
				.map((part) => part.trim())
				.filter(Boolean)
				.every((part) => /^-?\d+$/.test(part)),
		'chat_id'
	);

/** Heartbeat intervals are Go durations: `30m`, `1h`, `90s`. */
const durationSchema = z
	.string()
	.trim()
	.max(24, tooLong)
	.refine((value) => value === '' || /^\d+[smhd]$/.test(value), 'duration');

/**
 * The full onboarding form. It mirrors the rules `internal/onboard` enforces, so
 * the browser rejects the same input as `myaaw onboard` before the request is
 * sent. Empty optional fields mean "keep the current value" / "use the default".
 */
export const agentSetupSchema = z.object({
	name: nameSchema,
	provider_type: z.enum(['openai', 'gemini']),
	provider_name: z
		.string()
		.trim()
		.max(64, tooLong)
		.refine((value) => !/\s/.test(value), 'provider_name'),
	base_url: optionalUrlSchema,
	api_key: z.string().optional(),
	model: z.string().trim().min(1, required).max(256, tooLong),
	telegram_token: telegramTokenSchema,
	webhook_url: webhookUrlSchema,
	heartbeat_to: chatIdSchema,
	owner_ids: ownerIdsSchema,
	heartbeat_every: durationSchema
});

export type AgentSetupForm = z.infer<typeof agentSetupSchema>;
