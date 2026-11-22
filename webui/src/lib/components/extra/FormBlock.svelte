<script lang="ts">
	import { Check, CheckCircle2, Loader2 } from 'lucide-svelte';
	import Switch from '$lib/components/common/Switch.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import type {
		ExtraBlock,
		ExtraFormBlockData,
		ExtraFormField,
		InteractionRequest
	} from '$lib/types';

	interface Props {
		block: ExtraBlock;
		data: ExtraFormBlockData;
		disabled?: boolean;
		submitting?: boolean;
		onInteract?: (req: InteractionRequest) => Promise<void> | void;
	}

	let { block, data, disabled = false, submitting = false, onInteract }: Props = $props();

	const { t } = useTranslation();

	let singleUse = $derived(block.single_use !== false);
	let isSubmitted = $derived(block.state?.status === 'submitted');
	let isLocked = $derived((singleUse && isSubmitted) || disabled || submitting);

	function buildDefaults(fields: ExtraFormField[]): Record<string, unknown> {
		const out: Record<string, unknown> = {};
		for (const field of fields) {
			if (field.default !== undefined && field.default !== null) {
				out[field.name] = field.default;
			} else if (field.type === 'checkbox' || field.type === 'switch') {
				out[field.name] = false;
			} else if (field.type === 'multiselect') {
				out[field.name] = [];
			} else {
				out[field.name] = '';
			}
		}
		return out;
	}

	let values = $state<Record<string, unknown>>({});
	let errors = $state<Record<string, string>>({});
	let initializedFor = $state('');

	$effect(() => {
		const key = `${block.id}:${data.id ?? ''}`;
		if (initializedFor !== key) {
			initializedFor = key;
			values = buildDefaults(data.fields ?? []);
			errors = {};
		}
	});

	let submittedValues = $derived.by<Record<string, unknown>>(() => {
		const raw = block.state?.payload?.values;
		if (raw && typeof raw === 'object' && !Array.isArray(raw)) {
			return raw as Record<string, unknown>;
		}
		return {};
	});

	function setField(name: string, nextValue: unknown) {
		values = { ...values, [name]: nextValue };
		if (errors[name]) {
			const nextErrors = { ...errors };
			delete nextErrors[name];
			errors = nextErrors;
		}
	}

	function toggleMultiOption(name: string, optVal: string) {
		const current = Array.isArray(values[name]) ? (values[name] as string[]) : [];
		const next = current.includes(optVal)
			? current.filter((v) => v !== optVal)
			: [...current, optVal];
		setField(name, next);
	}

	function formatDisplayValue(field: ExtraFormField, raw: unknown): string {
		if (raw === undefined || raw === null || raw === '') return '—';
		if (typeof raw === 'boolean') return raw ? t('myaaw.extra.yes') : t('myaaw.extra.no');
		if (Array.isArray(raw)) {
			if (raw.length === 0) return '—';
			return raw
				.map((v) => field.options?.find((o) => o.value === String(v))?.label ?? String(v))
				.join(', ');
		}
		if (field.options?.length) {
			const found = field.options.find((o) => o.value === String(raw));
			if (found) return found.label;
		}
		return String(raw);
	}

	async function handleSubmit(event: SubmitEvent) {
		event.preventDefault();
		if (isLocked || !onInteract) return;

		const nextErrors: Record<string, string> = {};
		const cleanValues: Record<string, unknown> = {};
		const summaryParts: string[] = [];

		for (const field of data.fields ?? []) {
			const raw = values[field.name];
			if (field.type === 'number') {
				const str = String(raw ?? '').trim();
				if (str === '') {
					if (field.required) {
						nextErrors[field.name] = t('myaaw.extra.field_required');
					}
				} else {
					const num = Number(str);
					if (Number.isNaN(num)) {
						nextErrors[field.name] = t('myaaw.extra.invalid_number');
					} else if (field.min !== undefined && num < field.min) {
						nextErrors[field.name] = t('myaaw.extra.min_value', { min: field.min });
					} else if (field.max !== undefined && num > field.max) {
						nextErrors[field.name] = t('myaaw.extra.max_value', { max: field.max });
					} else {
						cleanValues[field.name] = num;
						summaryParts.push(`${field.label}: ${num}`);
					}
				}
			} else if (field.type === 'checkbox' || field.type === 'switch') {
				const boolVal = Boolean(raw);
				if (field.required && !boolVal) {
					nextErrors[field.name] = t('myaaw.extra.field_required');
				} else {
					cleanValues[field.name] = boolVal;
					summaryParts.push(
						`${field.label}: ${boolVal ? t('myaaw.extra.yes') : t('myaaw.extra.no')}`
					);
				}
			} else if (field.type === 'multiselect') {
				const arr = Array.isArray(raw) ? raw.map(String) : [];
				if (field.required && arr.length === 0) {
					nextErrors[field.name] = t('myaaw.extra.field_required');
				} else {
					cleanValues[field.name] = arr;
					if (arr.length > 0) {
						summaryParts.push(`${field.label}: ${formatDisplayValue(field, arr)}`);
					}
				}
			} else {
				const str = String(raw ?? '').trim();
				if (field.required && !str) {
					nextErrors[field.name] = t('myaaw.extra.field_required');
				} else {
					cleanValues[field.name] = str;
					if (str) {
						summaryParts.push(`${field.label}: ${formatDisplayValue(field, str)}`);
					}
				}
			}
		}

		errors = nextErrors;
		if (Object.keys(nextErrors).length > 0) return;

		const summary =
			summaryParts.length > 0 ? `${data.title}: ${summaryParts.join(' · ')}` : data.title;

		await onInteract({
			block_id: block.id,
			kind: 'form',
			action_id: data.id || block.id,
			label: data.title,
			values: cleanValues,
			display_text: summary
		});
	}
</script>

<div class="w-full max-w-lg overflow-hidden rounded-xl border bg-card">
	<div class="flex items-start justify-between gap-2 border-b px-3.5 py-2.5">
		<div class="min-w-0">
			<p class="text-sm font-semibold leading-snug text-foreground">{data.title}</p>
			{#if data.description}
				<p class="mt-0.5 text-xs text-muted-foreground">{data.description}</p>
			{/if}
		</div>
		{#if isSubmitted}
			<span
				class="inline-flex shrink-0 items-center gap-1 rounded-full bg-emerald-500/12 px-2 py-0.5 text-[11px] font-medium text-emerald-600 dark:text-emerald-400"
			>
				<CheckCircle2 class="h-3 w-3" />
				{t('myaaw.extra.submitted')}
			</span>
		{/if}
	</div>

	{#if isSubmitted && singleUse}
		<dl class="divide-y divide-border/60">
			{#each data.fields ?? [] as field (field.name)}
				<div class="flex items-baseline justify-between gap-4 px-3.5 py-2 text-xs">
					<dt class="text-muted-foreground">{field.label}</dt>
					<dd class="text-right font-medium text-foreground wrap-break-word">
						{formatDisplayValue(field, submittedValues[field.name])}
					</dd>
				</div>
			{/each}
		</dl>
	{:else}
		<form onsubmit={handleSubmit} class="space-y-3 p-3.5">
			{#each data.fields ?? [] as field (field.name)}
				{@const fieldId = `${block.id}-${field.name}`}
				<div class="space-y-1">
					{#if field.type !== 'checkbox' && field.type !== 'switch'}
						<label for={fieldId} class="block text-xs font-medium text-foreground">
							{field.label}
							{#if field.required}
								<span class="text-destructive">*</span>
							{/if}
						</label>
					{/if}

					{#if field.type === 'textarea'}
						<textarea
							id={fieldId}
							rows="3"
							disabled={isLocked}
							placeholder={field.placeholder ?? ''}
							value={String(values[field.name] ?? '')}
							oninput={(e) => setField(field.name, e.currentTarget.value)}
							class="w-full rounded-lg border border-input bg-background px-2.5 py-1.5 text-xs text-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring/40 disabled:opacity-60"
						></textarea>
					{:else if field.type === 'number'}
						<input
							id={fieldId}
							type="number"
							min={field.min}
							max={field.max}
							step="any"
							disabled={isLocked}
							placeholder={field.placeholder ?? ''}
							value={String(values[field.name] ?? '')}
							oninput={(e) => setField(field.name, e.currentTarget.value)}
							class="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-xs text-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring/40 disabled:opacity-60"
						/>
					{:else if field.type === 'date'}
						<input
							id={fieldId}
							type="date"
							disabled={isLocked}
							value={String(values[field.name] ?? '')}
							oninput={(e) => setField(field.name, e.currentTarget.value)}
							class="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-xs text-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring/40 disabled:opacity-60"
						/>
					{:else if field.type === 'time'}
						<input
							id={fieldId}
							type="time"
							disabled={isLocked}
							value={String(values[field.name] ?? '')}
							oninput={(e) => setField(field.name, e.currentTarget.value)}
							class="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-xs text-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring/40 disabled:opacity-60"
						/>
					{:else if field.type === 'select'}
						<select
							id={fieldId}
							disabled={isLocked}
							value={String(values[field.name] ?? '')}
							onchange={(e) => setField(field.name, e.currentTarget.value)}
							class="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-xs text-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring/40 disabled:opacity-60"
						>
							<option value="">{field.placeholder || t('myaaw.extra.select_placeholder')}</option>
							{#each field.options ?? [] as opt (opt.value)}
								<option value={opt.value}>{opt.label}</option>
							{/each}
						</select>
					{:else if field.type === 'radio'}
						<div class="flex flex-wrap gap-1.5 pt-0.5">
							{#each field.options ?? [] as opt (opt.value)}
								{@const active = String(values[field.name] ?? '') === opt.value}
								<button
									type="button"
									disabled={isLocked}
									onclick={() => setField(field.name, opt.value)}
									class={[
										'inline-flex items-center gap-1.5 rounded-lg border px-2.5 py-1 text-xs font-medium transition-colors',
										active
											? 'border-primary bg-primary/12 text-primary'
											: 'border-border bg-background text-foreground hover:bg-muted/60',
										isLocked && 'cursor-not-allowed opacity-60'
									]}
								>
									{#if active}
										<Check class="h-3 w-3" />
									{/if}
									<span>{opt.label}</span>
								</button>
							{/each}
						</div>
					{:else if field.type === 'multiselect'}
						{@const selected = Array.isArray(values[field.name])
							? (values[field.name] as string[])
							: []}
						<div class="flex flex-wrap gap-1.5 pt-0.5">
							{#each field.options ?? [] as opt (opt.value)}
								{@const active = selected.includes(opt.value)}
								<button
									type="button"
									disabled={isLocked}
									onclick={() => toggleMultiOption(field.name, opt.value)}
									class={[
										'inline-flex items-center gap-1.5 rounded-lg border px-2.5 py-1 text-xs font-medium transition-colors',
										active
											? 'border-primary bg-primary/12 text-primary'
											: 'border-border bg-background text-foreground hover:bg-muted/60',
										isLocked && 'cursor-not-allowed opacity-60'
									]}
								>
									{#if active}
										<Check class="h-3 w-3" />
									{/if}
									<span>{opt.label}</span>
								</button>
							{/each}
						</div>
					{:else if field.type === 'switch'}
						<div class="flex items-center justify-between gap-3 py-0.5">
							<span class="text-xs font-medium text-foreground">
								{field.label}
								{#if field.required}
									<span class="text-destructive">*</span>
								{/if}
							</span>
							<Switch
								checked={Boolean(values[field.name])}
								label={field.label}
								disabled={isLocked}
								onchange={(checked) => setField(field.name, checked)}
							/>
						</div>
					{:else if field.type === 'checkbox'}
						<label class="flex cursor-pointer items-center gap-2 py-0.5 text-xs text-foreground">
							<input
								id={fieldId}
								type="checkbox"
								disabled={isLocked}
								checked={Boolean(values[field.name])}
								onchange={(e) => setField(field.name, e.currentTarget.checked)}
								class="h-4 w-4 rounded border-input accent-primary"
							/>
							<span class="font-medium">
								{field.label}
								{#if field.required}
									<span class="text-destructive">*</span>
								{/if}
							</span>
						</label>
					{:else}
						<input
							id={fieldId}
							type="text"
							disabled={isLocked}
							placeholder={field.placeholder ?? ''}
							value={String(values[field.name] ?? '')}
							oninput={(e) => setField(field.name, e.currentTarget.value)}
							class="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-xs text-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring/40 disabled:opacity-60"
						/>
					{/if}

					{#if field.help}
						<p class="text-[11px] text-muted-foreground">{field.help}</p>
					{/if}
					{#if errors[field.name]}
						<p class="text-[11px] text-destructive">{errors[field.name]}</p>
					{/if}
				</div>
			{/each}

			<div class="flex justify-end pt-1">
				<button
					type="submit"
					disabled={isLocked}
					class="inline-flex items-center gap-1.5 rounded-lg bg-primary px-3.5 py-1.5 text-xs font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:cursor-not-allowed disabled:opacity-55"
				>
					{#if submitting}
						<Loader2 class="h-3.5 w-3.5 animate-spin" />
					{/if}
					<span>{data.submit_label || t('myaaw.extra.submit')}</span>
				</button>
			</div>
		</form>
	{/if}
</div>
