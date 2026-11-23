<script lang="ts">
	import { CornerDownRight, ShieldAlert } from 'lucide-svelte';
	import { Button } from '$lib/components/ui/button';
	import { groupAccent } from '$lib/components/groups/accent';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import type { ApprovalDecision, ApprovalPrompt } from '$lib/types';

	interface Props {
		approval: ApprovalPrompt;
		onDecide: (decision: ApprovalDecision) => void;
		/** Optional agent name when rendered inside a multi-agent group chat. */
		agentName?: string;
		agentColor?: string;
		/** Optional 1-based queue indicator when multiple approvals are waiting. */
		queuePosition?: number;
		queueTotal?: number;
	}

	let { approval, onDecide, agentName, agentColor, queuePosition, queueTotal }: Props = $props();
	const { t } = useTranslation();

	const agentBadge = $derived(agentName ? groupAccent(agentColor) : null);

	/** The arguments start collapsed, like the CLI prompt; this opens the full JSON. */
	let openArgs = $state(false);

	/** The summary is a 200-character preview, so the full JSON is what shows details. */
	const fullArgs = $derived(pretty(approval.args));
	const canExpand = $derived(fullArgs.length > 0 && fullArgs !== (approval.args_summary ?? ''));

	function pretty(raw?: string): string {
		if (!raw) return '';
		try {
			return JSON.stringify(JSON.parse(raw), null, 2);
		} catch {
			return raw;
		}
	}
</script>

<!--
	Pinned above the composer instead of inline in the transcript: the run is
	paused while it waits, and the decision should not push the answer around or
	scroll away with it.
-->
<div
	class="rounded-xl border border-amber-500/40 bg-amber-500/10 p-3 text-xs shadow-sm"
	role="alert"
>
	<div class="flex flex-wrap items-center gap-2 font-medium text-amber-700 dark:text-amber-300">
		<ShieldAlert class="h-4 w-4 shrink-0" />
		<span>{t('myaaw.chat.need_approval')}</span>
		{#if agentName && agentBadge}
			<span
				class={[
					'inline-flex items-center gap-1 rounded-full px-1.5 py-0.5 text-[10px] font-semibold ring-1',
					agentBadge.softBg,
					agentBadge.text,
					agentBadge.ring
				]}
			>
				<span class={['h-1.5 w-1.5 rounded-full', agentBadge.dot]}></span>
				@{agentName}
			</span>
		{/if}
		{#if approval.subagent}
			<!-- The task name alone reads like part of the tool; say where it came from. -->
			<span
				class="rounded-full bg-amber-500/25 px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-amber-800 dark:text-amber-100"
			>
				{t('myaaw.chat.trigger_subagent')}
			</span>
		{/if}
		<span class="truncate font-mono text-foreground">{approval.tool_name}</span>
		{#if queueTotal && queueTotal > 1}
			<span
				class="ml-auto rounded-full bg-amber-500/20 px-1.5 py-0.5 text-[10px] font-semibold tabular-nums text-amber-800 dark:text-amber-200"
			>
				{queuePosition ?? 1} / {queueTotal}
			</span>
		{/if}
	</div>
	{#if approval.subagent}
		<p class="mt-1 flex flex-wrap items-center gap-1.5 text-[11px]">
			<CornerDownRight class="h-3.5 w-3.5 shrink-0 text-amber-700 dark:text-amber-300" />
			<span class="text-amber-700 dark:text-amber-300">
				{t('myaaw.chat.from_subagent')}
			</span>
			<span class="rounded bg-background/60 px-1.5 py-0.5 font-mono text-[10px] text-foreground">
				{approval.subagent}
			</span>
		</p>
	{/if}
	{#if openArgs && fullArgs}
		<pre
			class="mt-1 max-h-64 overflow-auto whitespace-pre-wrap break-all font-mono text-[11px] text-muted-foreground">{fullArgs}</pre>
	{:else if approval.args_summary}
		<pre
			class="mt-1 max-h-24 overflow-auto whitespace-pre-wrap break-all font-mono text-[11px] text-muted-foreground">{approval.args_summary}</pre>
	{/if}
	{#if canExpand}
		<button
			type="button"
			class="mt-1 text-[10px] font-medium text-muted-foreground underline underline-offset-2 hover:text-foreground"
			aria-expanded={openArgs}
			onclick={() => (openArgs = !openArgs)}
		>
			{openArgs ? t('myaaw.chat.hide_args') : t('myaaw.chat.show_args')}
		</button>
	{/if}
	<div class="mt-2 flex flex-wrap gap-2">
		<Button size="xs" onclick={() => onDecide('allow_once')}>
			{t('myaaw.chat.allow_once')}
		</Button>
		<Button size="xs" variant="secondary" onclick={() => onDecide('allow_session')}>
			{t('myaaw.chat.allow_session')}
		</Button>
		<Button size="xs" variant="secondary" onclick={() => onDecide('allow_always')}>
			{t('myaaw.chat.allow_always')}
		</Button>
		<Button size="xs" variant="destructive" onclick={() => onDecide('deny')}>
			{t('myaaw.chat.deny')}
		</Button>
	</div>
</div>
