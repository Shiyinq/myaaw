<script lang="ts">
	import { untrack } from 'svelte';
	import { CalendarClock, Loader2, Plus } from 'lucide-svelte';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import FieldError from '$lib/components/common/FieldError.svelte';
	import Switch from '$lib/components/common/Switch.svelte';
	import { cronStore } from '$lib/stores/cron.svelte';
	import { conversationsStore } from '$lib/stores/conversations.svelte';
	import { toastStore } from '$lib/stores';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { getErrorMessage } from '$lib/utils/api';
	import { validate, type FieldErrors } from '$lib/utils/validation';
	import { cronJobSchema } from '$lib/schemas';
	import type { ConversationSummary, CronChannel, CronChatType } from '$lib/types';

	interface Props {
		open: boolean;
		agent: string;
		onOpenChange: (open: boolean) => void;
		onCreated?: () => void;
	}

	let { open, agent, onOpenChange, onCreated }: Props = $props();
	const { t, locale } = useTranslation();

	let saving = $state(false);
	let formError = $state('');

	let name = $state('');
	let kind = $state<'cron' | 'every' | 'at'>('cron');
	let expr = $state('0 7 * * *');
	let tz = $state('Asia/Jakarta');
	let message = $state('');
	let channel = $state<CronChannel>('telegram');
	let to = $state('');
	let threadId = $state('');
	let sessionId = $state('');
	let chatType = $state<CronChatType>('guild');
	let enabled = $state(true);
	let errors = $state<FieldErrors>({});

	// The preset is a frontend convenience: it renders the cron expression the
	// backend already understands, so nothing new is stored.
	let preset = $state('custom');
	let presetMinutes = $state(15);
	let presetTime = $state('07:00');
	let presetWeekday = $state('1');
	let presetDay = $state('1');

	// Session history for the picker, loaded lazily only while the dialog is open.
	let sessionChoices = $state<ConversationSummary[]>([]);
	let sessionChoicesLoading = $state(false);
	let sessionsKey = '';

	const isTelegram = $derived(channel === 'telegram');
	const isDiscord = $derived(channel === 'discord');
	const needsSession = $derived(channel === 'api' || channel === 'cli');

	const kindOptions: { id: 'cron' | 'every' | 'at'; labelKey: string; example: string }[] = [
		{ id: 'cron', labelKey: 'myaaw.schedule.kind_recurring', example: '0 7 * * *' },
		{ id: 'every', labelKey: 'myaaw.schedule.kind_interval', example: '30m / 2h' },
		{ id: 'at', labelKey: 'myaaw.schedule.kind_once', example: '2026-05-01T09:00' }
	];

	const channelOptions: { id: CronChannel; labelKey: string }[] = [
		{ id: 'telegram', labelKey: 'myaaw.schedule.channel_telegram' },
		{ id: 'discord', labelKey: 'myaaw.schedule.channel_discord' },
		{ id: 'api', labelKey: 'myaaw.schedule.channel_api' },
		{ id: 'cli', labelKey: 'myaaw.schedule.channel_cli' }
	];

	const weekdays = $derived(
		Array.from({ length: 7 }, (_, index) => ({
			value: String(index),
			label: new Intl.DateTimeFormat(locale.value, {
				weekday: 'long',
				timeZone: 'UTC'
			}).format(new Date(Date.UTC(2023, 0, 1 + index)))
		}))
	);

	function presetTimeParts() {
		const [hour, minute] = presetTime.split(':').map((part) => Number.parseInt(part, 10));
		return {
			hour: Number.isFinite(hour) ? Math.min(23, Math.max(0, hour)) : 0,
			minute: Number.isFinite(minute) ? Math.min(59, Math.max(0, minute)) : 0
		};
	}

	/** The cron expression a preset expands to; empty for the custom mode. */
	const presetCron = $derived.by(() => {
		const { hour, minute } = presetTimeParts();
		switch (preset) {
			case 'hourly':
				return '0 * * * *';
			case 'daily':
				return `${minute} ${hour} * * *`;
			case 'weekdays':
				return `${minute} ${hour} * * 1-5`;
			case 'weekly':
				return `${minute} ${hour} * * ${presetWeekday}`;
			case 'monthly':
				return `${minute} ${hour} ${presetDay} * *`;
			case 'minutes': {
				const every = Math.min(59, Math.max(1, Number(presetMinutes) || 1));
				return `*/${every} * * * *`;
			}
			default:
				return '';
		}
	});

	// Reset form cleanly whenever the dialog opens.
	$effect(() => {
		if (!open) return;
		untrack(() => {
			name = '';
			kind = 'cron';
			expr = '0 7 * * *';
			tz = 'Asia/Jakarta';
			message = '';
			channel = 'telegram';
			to = '';
			threadId = '';
			sessionId = '';
			chatType = 'guild';
			enabled = true;
			preset = 'custom';
			presetMinutes = 15;
			presetTime = '07:00';
			presetWeekday = '1';
			presetDay = '1';
			errors = {};
			formError = '';
			saving = false;
			sessionsKey = '';
		});
	});

	async function loadSessions(currentAgent: string, currentChannel: CronChannel) {
		sessionChoicesLoading = true;
		try {
			const page = await conversationsStore.fetchList(currentAgent, {
				limit: 50,
				channel: currentChannel
			});
			sessionChoices = page.conversations ?? [];
		} catch {
			sessionChoices = [];
		} finally {
			sessionChoicesLoading = false;
		}
	}

	$effect(() => {
		if (!open || !agent) return;
		const key = `${agent}:${channel}`;
		if (key === sessionsKey) return;
		sessionsKey = key;
		void loadSessions(agent, channel);
	});

	function changeChannel(next: CronChannel) {
		if (channel === next) return;
		channel = next;
		to = '';
		threadId = '';
		sessionId = '';
		chatType = 'guild';
		errors = {};
	}

	function pickSession(id: string) {
		const choice = sessionChoices.find((candidate) => candidate.id === id);
		if (!choice) {
			sessionId = '';
			return;
		}
		sessionId = choice.id;
		if (isTelegram) {
			const parts = (choice.scope ?? '').split(':');
			threadId = parts.length >= 3 ? parts[2] : '';
		}
		if (isDiscord) {
			chatType = (choice.scope ?? '').startsWith('dc:') ? 'guild' : 'dm';
		}
	}

	async function handleSubmit(event: SubmitEvent) {
		event.preventDefault();
		if (!agent || saving) return;

		const submitted = kind === 'cron' && preset !== 'custom' ? presetCron : expr;
		const result = validate(cronJobSchema, {
			name,
			schedule_kind: kind,
			expr: submitted,
			tz,
			message,
			channel,
			to,
			thread_id: isTelegram ? threadId.trim() : '',
			session_id: sessionId.trim(),
			chat_type: isDiscord ? chatType : undefined
		});
		if (!result.ok) {
			errors = result.errors;
			return;
		}

		errors = {};
		formError = '';
		saving = true;
		try {
			await cronStore.add(agent, {
				name: result.data.name,
				schedule_kind: result.data.schedule_kind,
				expr: result.data.expr,
				tz: result.data.tz,
				message: result.data.message,
				channel: result.data.channel,
				to: result.data.to,
				thread_id: isTelegram ? result.data.thread_id || undefined : undefined,
				session_id: result.data.session_id || undefined,
				chat_type: isDiscord ? result.data.chat_type : undefined,
				enabled
			});
			toastStore.success(t('myaaw.schedule.saved'));
			onOpenChange(false);
			onCreated?.();
		} catch (err) {
			formError = getErrorMessage(err);
		} finally {
			saving = false;
		}
	}
</script>

<Dialog.Root {open} {onOpenChange}>
	<Dialog.Content
		class="inset-0 top-0 left-0 flex h-dvh max-h-dvh w-screen max-w-none translate-x-0 translate-y-0 flex-col gap-0 overflow-hidden rounded-none bg-background p-0 ring-0 sm:inset-auto sm:top-1/2 sm:left-1/2 sm:h-auto sm:max-h-[90vh] sm:w-full sm:max-w-lg sm:-translate-x-1/2 sm:-translate-y-1/2 sm:rounded-xl sm:bg-popover sm:ring-1"
	>
		<Dialog.Header
			class="shrink-0 border-b px-4 py-3.5 pr-12 text-left sm:border-b-0 sm:px-5 sm:pt-5 sm:pb-2"
		>
			<Dialog.Title class="flex items-center gap-2">
				<CalendarClock class="h-4 w-4 text-primary" />
				{t('myaaw.schedule.create')}
			</Dialog.Title>
			<Dialog.Description>{t('myaaw.schedule.create_desc')}</Dialog.Description>
		</Dialog.Header>

		<form
			onsubmit={handleSubmit}
			autocomplete="off"
			class="flex min-h-0 flex-1 flex-col overflow-hidden"
		>
			<div
				class="min-h-0 flex-1 space-y-4 overflow-y-auto overscroll-contain px-4 py-4 sm:px-5 sm:py-2"
			>
				<!-- Name -->
				<div class="space-y-1.5">
					<Label for="cron-name">{t('myaaw.schedule.name')}</Label>
					<Input id="cron-name" bind:value={name} placeholder="Morning reminder" />
					<FieldError code={errors.name} />
				</div>

				<!-- Schedule Kind (3-column card picker matching CreateGroupDialog) -->
				<div class="space-y-2">
					<Label>{t('myaaw.schedule.kind')}</Label>
					<div class="grid grid-cols-1 gap-1.5 sm:grid-cols-3">
						{#each kindOptions as opt (opt.id)}
							{@const active = kind === opt.id}
							<button
								type="button"
								onclick={() => (kind = opt.id)}
								aria-pressed={active}
								class={[
									'flex flex-col rounded-xl border p-2.5 text-left transition-colors',
									active
										? 'border-primary bg-primary/8 text-foreground'
										: 'border-border hover:bg-muted/60'
								]}
							>
								<span class="text-xs font-semibold">{t(opt.labelKey)}</span>
								<span class="mt-0.5 font-mono text-[10px] text-muted-foreground">{opt.example}</span
								>
							</button>
						{/each}
					</div>
				</div>

				<!-- Timing details -->
				<div class="space-y-3 rounded-xl border p-3">
					{#if kind === 'cron'}
						<div class="grid grid-cols-1 gap-2.5 sm:grid-cols-2">
							<div class="space-y-1.5">
								<Label for="cron-preset" class="text-xs">{t('myaaw.schedule.preset')}</Label>
								<select
									id="cron-preset"
									bind:value={preset}
									class="h-9 w-full rounded-md border border-input bg-transparent px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50"
								>
									<option value="custom">{t('myaaw.schedule.preset_custom')}</option>
									<option value="minutes">{t('myaaw.schedule.preset_minutes')}</option>
									<option value="hourly">{t('myaaw.schedule.preset_hourly')}</option>
									<option value="daily">{t('myaaw.schedule.preset_daily')}</option>
									<option value="weekdays">{t('myaaw.schedule.preset_weekdays')}</option>
									<option value="weekly">{t('myaaw.schedule.preset_weekly')}</option>
									<option value="monthly">{t('myaaw.schedule.preset_monthly')}</option>
								</select>
							</div>
							<div class="space-y-1.5">
								<Label for="cron-tz" class="text-xs">{t('myaaw.schedule.tz')}</Label>
								<Input id="cron-tz" bind:value={tz} />
							</div>
						</div>

						{#if preset === 'minutes'}
							<div class="space-y-1.5">
								<Label for="cron-minutes" class="text-xs">{t('myaaw.schedule.minutes')}</Label>
								<Input
									id="cron-minutes"
									type="number"
									min="1"
									max="59"
									bind:value={presetMinutes}
								/>
							</div>
						{:else if preset === 'daily' || preset === 'weekdays' || preset === 'weekly' || preset === 'monthly'}
							<div class="grid grid-cols-1 gap-2.5 sm:grid-cols-2">
								<div class="space-y-1.5">
									<Label for="cron-time" class="text-xs">{t('myaaw.schedule.time')}</Label>
									<Input id="cron-time" type="time" bind:value={presetTime} />
								</div>
								{#if preset === 'weekly'}
									<div class="space-y-1.5">
										<Label for="cron-weekday" class="text-xs">{t('myaaw.schedule.weekday')}</Label>
										<select
											id="cron-weekday"
											bind:value={presetWeekday}
											class="h-9 w-full rounded-md border border-input bg-transparent px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50"
										>
											{#each weekdays as day (day.value)}
												<option value={day.value}>{day.label}</option>
											{/each}
										</select>
									</div>
								{:else if preset === 'monthly'}
									<div class="space-y-1.5">
										<Label for="cron-day" class="text-xs">{t('myaaw.schedule.day_of_month')}</Label>
										<Input id="cron-day" type="number" min="1" max="31" bind:value={presetDay} />
									</div>
								{/if}
							</div>
						{/if}

						<div class="space-y-1.5">
							<Label for="cron-expr" class="text-xs">{t('myaaw.schedule.expr')}</Label>
							{#if preset === 'custom'}
								<Input
									id="cron-expr"
									bind:value={expr}
									placeholder="0 7 * * *"
									class="font-mono text-xs"
								/>
							{:else}
								<Input id="cron-expr" value={presetCron} disabled class="font-mono text-xs" />
							{/if}
							<FieldError code={errors.expr} />
						</div>
					{:else}
						<div class="grid grid-cols-1 gap-2.5 sm:grid-cols-2">
							<div class="space-y-1.5">
								<Label for="cron-expr" class="text-xs">{t('myaaw.schedule.expr')}</Label>
								<Input
									id="cron-expr"
									bind:value={expr}
									placeholder={kind === 'every' ? '30m' : '2026-05-01T09:00:00+07:00'}
									class="font-mono text-xs"
								/>
								<FieldError code={errors.expr} />
							</div>
							<div class="space-y-1.5">
								<Label for="cron-tz" class="text-xs">{t('myaaw.schedule.tz')}</Label>
								<Input id="cron-tz" bind:value={tz} />
							</div>
						</div>
					{/if}
				</div>

				<!-- Prompt -->
				<div class="space-y-1.5">
					<Label for="cron-message">{t('myaaw.schedule.message')}</Label>
					<textarea
						id="cron-message"
						bind:value={message}
						rows={3}
						placeholder="Good morning!"
						class="w-full resize-none rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-xs outline-none placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50"
					></textarea>
					<FieldError code={errors.message} />
				</div>

				<!-- Delivery channel & target -->
				<div class="space-y-3 rounded-xl border p-3">
					<div class="space-y-2">
						<Label class="text-xs">{t('myaaw.schedule.section_delivery')}</Label>
						<div class="grid grid-cols-2 gap-1.5 sm:grid-cols-4">
							{#each channelOptions as opt (opt.id)}
								{@const active = channel === opt.id}
								<button
									type="button"
									onclick={() => changeChannel(opt.id)}
									aria-pressed={active}
									class={[
										'rounded-lg border px-2.5 py-1.5 text-center text-xs font-medium capitalize transition-colors',
										active
											? 'border-primary bg-primary/8 text-foreground'
											: 'border-border text-muted-foreground hover:bg-muted/60 hover:text-foreground'
									]}
								>
									{t(opt.labelKey)}
								</button>
							{/each}
						</div>
					</div>

					<div class="space-y-1.5">
						<Label for="cron-to" class="text-xs">{t('myaaw.schedule.to')}</Label>
						<Input id="cron-to" bind:value={to} placeholder="123456789" />
						<p class="text-[11px] text-muted-foreground">
							{#if isTelegram}
								{t('myaaw.schedule.to_telegram_hint')}
							{:else if isDiscord}
								{t('myaaw.schedule.to_discord_hint')}
							{:else}
								{t('myaaw.schedule.to_api_hint')}
							{/if}
						</p>
					</div>

					{#if isTelegram}
						<div class="space-y-1.5">
							<Label for="cron-thread" class="text-xs">{t('myaaw.schedule.thread_id')}</Label>
							<Input id="cron-thread" bind:value={threadId} placeholder="77" />
							<p class="text-[11px] text-muted-foreground">
								{t('myaaw.schedule.thread_id_hint')}
							</p>
							<FieldError code={errors.thread_id} />
						</div>
					{:else if isDiscord}
						<div class="space-y-1.5">
							<Label for="cron-chat-type" class="text-xs">{t('myaaw.schedule.chat_type')}</Label>
							<select
								id="cron-chat-type"
								bind:value={chatType}
								class="h-9 w-full rounded-md border border-input bg-transparent px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50"
							>
								<option value="guild">{t('myaaw.schedule.chat_type_guild')}</option>
								<option value="dm">{t('myaaw.schedule.chat_type_dm')}</option>
							</select>
							<p class="text-[11px] text-muted-foreground">
								{t('myaaw.schedule.chat_type_hint')}
							</p>
						</div>
					{/if}

					<div class="space-y-1.5">
						<Label for="cron-session" class="text-xs">{t('myaaw.schedule.session_id')}</Label>
						<select
							id="cron-session-pick"
							value={sessionChoices.some((choice) => choice.id === sessionId) ? sessionId : ''}
							onchange={(event) => pickSession(event.currentTarget.value)}
							disabled={sessionChoicesLoading}
							class="h-9 w-full rounded-md border border-input bg-transparent px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50"
						>
							<option value="">
								{sessionChoices.length === 0
									? t('myaaw.schedule.session_none')
									: t('myaaw.schedule.session_history')}
							</option>
							{#each sessionChoices as choice (choice.id)}
								<option value={choice.id}>
									{choice.title || choice.id.slice(0, 8)} · {choice.id.slice(0, 8)}
								</option>
							{/each}
						</select>
						<Input
							id="cron-session"
							bind:value={sessionId}
							placeholder="0f4e8b3a-…"
							class="font-mono text-xs"
						/>
						<p class="text-[11px] text-muted-foreground">
							{needsSession
								? t('myaaw.schedule.session_id_required_hint')
								: t('myaaw.schedule.session_id_optional_hint')}
						</p>
						<FieldError code={errors.session_id} />
					</div>
				</div>

				<!-- Enabled switch row -->
				<div class="flex items-center justify-between gap-3 rounded-xl border px-3 py-2.5">
					<div class="min-w-0">
						<p class="text-xs font-medium capitalize">{t('myaaw.schedule.enabled')}</p>
						<p class="text-[11px] text-muted-foreground">{t('myaaw.schedule.jobs_desc')}</p>
					</div>
					<Switch
						checked={enabled}
						label={t('myaaw.schedule.enabled')}
						onchange={(next) => (enabled = next)}
					/>
				</div>

				{#if formError}
					<p
						class="rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 text-xs text-destructive"
					>
						{formError}
					</p>
				{/if}
			</div>

			<Dialog.Footer
				class="mx-0 mb-0 shrink-0 flex-row justify-end gap-2 rounded-none border-t bg-muted/40 px-4 py-3.5 sm:rounded-b-xl sm:px-5 sm:py-3.5"
			>
				<Button
					type="button"
					variant="outline"
					onclick={() => onOpenChange(false)}
					disabled={saving}
				>
					{t('common.cancel')}
				</Button>
				<Button type="submit" disabled={saving}>
					{#if saving}
						<Loader2 class="mr-1.5 h-3.5 w-3.5 animate-spin" />
					{:else}
						<Plus class="mr-1.5 h-3.5 w-3.5" />
					{/if}
					{t('myaaw.schedule.create')}
				</Button>
			</Dialog.Footer>
		</form>
	</Dialog.Content>
</Dialog.Root>
