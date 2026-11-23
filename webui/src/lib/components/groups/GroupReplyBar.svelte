<script lang="ts">
	import { Reply, UserRound, X } from 'lucide-svelte';
	import Bloub from '$lib/components/common/Bloub.svelte';
	import { colorOf } from '$lib/bloub';
	import { groupAccent } from './accent';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import type { GroupMessage } from '$lib/types';

	interface Props {
		message: GroupMessage;
		agentColor?: string;
		onclear: () => void;
	}

	let { message, agentColor, onclear }: Props = $props();

	const { t } = useTranslation();

	const isAgent = $derived(message.sender_type === 'agent');
	const resolvedColor = $derived(agentColor || message.agent_color);
	const accent = $derived(isAgent ? groupAccent(resolvedColor) : null);
	const senderLabel = $derived(
		message.sender_type === 'user'
			? t('myaaw.groups.you')
			: message.sender_name || t('myaaw.groups.system')
	);
	const excerpt = $derived(message.content.replace(/\s+/g, ' ').trim());
</script>

<div
	data-group-reply-bar
	class={[
		'flex items-start gap-2.5 rounded-xl border border-l-[3px] bg-muted/40 px-3 py-2 text-xs',
		accent ? accent.border : 'border-l-primary'
	]}
>
	<div class="mt-0.5 shrink-0">
		{#if isAgent}
			<Bloub mood="attentif" size={20} tight color={colorOf(resolvedColor)} />
		{:else}
			<span class="grid h-5 w-5 place-items-center rounded-full bg-primary/15 text-primary">
				<UserRound class="h-3 w-3" />
			</span>
		{/if}
	</div>

	<div class="min-w-0 flex-1">
		<div class="flex items-center gap-1.5">
			<Reply class="h-3 w-3 shrink-0 text-muted-foreground" />
			<span class={['truncate font-semibold', accent ? accent.text : 'text-primary']}>
				{t('myaaw.groups.replying_to', { name: senderLabel })}
			</span>
		</div>
		<p class="mt-0.5 line-clamp-2 text-muted-foreground">
			{excerpt}
		</p>
	</div>

	<button
		type="button"
		onclick={onclear}
		title={t('myaaw.groups.cancel_reply')}
		aria-label={t('myaaw.groups.cancel_reply')}
		class="grid h-6 w-6 shrink-0 place-items-center rounded-full text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
	>
		<X class="h-3.5 w-3.5" />
	</button>
</div>
