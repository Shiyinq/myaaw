<script lang="ts">
	import { ShieldAlert } from 'lucide-svelte';
	import { Button } from '$lib/components/ui/button';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import type { ApprovalDecision, ApprovalPrompt } from '$lib/types';

	interface Props {
		approval: ApprovalPrompt;
		onDecide: (decision: ApprovalDecision) => void;
	}

	let { approval, onDecide }: Props = $props();
	const { t } = useTranslation();
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
		<span class="truncate font-mono text-foreground">{approval.tool_name}</span>
		{#if approval.subagent}
			<span class="text-muted-foreground">· {approval.subagent}</span>
		{/if}
	</div>
	{#if approval.args_summary}
		<pre
			class="mt-1 max-h-24 overflow-auto whitespace-pre-wrap break-all font-mono text-[11px] text-muted-foreground">{approval.args_summary}</pre>
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
