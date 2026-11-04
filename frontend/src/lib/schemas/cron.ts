import { z } from 'zod';
import { invalidCron, isValidCron, nameSchema, optionalTextSchema, required } from './primitives';

export const cronChannelSchema = z.enum(['telegram', 'discord', 'api', 'cli']);
export const cronChatTypeSchema = z.enum(['guild', 'dm']);

const baseCronSchema = z.object({
	name: nameSchema,
	schedule_kind: z.enum(['cron', 'every', 'at']),
	expr: z.string().trim().min(1, required),
	tz: optionalTextSchema.optional(),
	message: z.string().trim().min(1, required),
	channel: cronChannelSchema,
	to: z.string().trim().min(1, required),
	thread_id: optionalTextSchema.optional(),
	session_id: optionalTextSchema.optional(),
	chat_type: cronChatTypeSchema.optional()
});

export const cronJobSchema = baseCronSchema
	.refine((job) => job.schedule_kind !== 'cron' || isValidCron(job.expr), {
		path: ['expr'],
		message: invalidCron
	})
	// A web or terminal target has no platform room id, so the session is the
	// only way to say which conversation the job belongs to.
	.refine((job) => (job.channel !== 'api' && job.channel !== 'cli') || !!job.session_id?.trim(), {
		path: ['session_id'],
		message: required
	})
	.refine((job) => job.channel !== 'telegram' || !job.thread_id || /^\d+$/.test(job.thread_id), {
		path: ['thread_id'],
		message: 'thread_id'
	});

export type CronJobForm = z.infer<typeof baseCronSchema>;
