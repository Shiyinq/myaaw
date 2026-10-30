import { z } from 'zod';
import { invalidCron, isValidCron, nameSchema, optionalTextSchema, required } from './primitives';

const baseCronSchema = z.object({
	name: nameSchema,
	schedule_kind: z.enum(['cron', 'every', 'at']),
	expr: z.string().trim().min(1, required),
	tz: optionalTextSchema.optional(),
	message: z.string().trim().min(1, required),
	channel: optionalTextSchema.optional(),
	to: optionalTextSchema.optional()
});

export const cronJobSchema = baseCronSchema.refine(
	(job) => job.schedule_kind !== 'cron' || isValidCron(job.expr),
	{ path: ['expr'], message: invalidCron }
);

export type CronJobForm = z.infer<typeof baseCronSchema>;
