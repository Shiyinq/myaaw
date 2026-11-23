<script lang="ts">
	import { Loader2, MoreHorizontal } from 'lucide-svelte';
	import AgentMoreMenuItems from '$lib/components/app/AgentMoreMenuItems.svelte';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { agentSelection } from '$lib/stores';
	import { useTranslation } from '$lib/i18n/useTranslation';

	interface Props {
		/** Called after picking a destination so the mobile drawer closes. */
		onNavigate?: () => void;
		/** Alignment of the dropdown content. */
		side?: 'bottom' | 'right' | 'top' | 'left';
		align?: 'start' | 'center' | 'end';
	}

	let { onNavigate, side = 'bottom', align = 'start' }: Props = $props();

	const { t } = useTranslation();

	let busy = $state(false);

	const agent = $derived(agentSelection.selected);
	const running = $derived(Boolean(agentSelection.current?.running));
</script>

<DropdownMenu.Root>
	<DropdownMenu.Trigger>
		{#snippet child({ props })}
			<button
				{...props}
				type="button"
				title={t('myaaw.bots.actions')}
				aria-label={t('myaaw.bots.actions')}
				class="flex h-9 w-9 shrink-0 items-center justify-center rounded-md border text-muted-foreground transition-colors outline-none hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/60"
			>
				{#if busy}
					<Loader2 class="h-4 w-4 animate-spin" />
				{:else}
					<MoreHorizontal class="h-4 w-4" />
				{/if}
			</button>
		{/snippet}
	</DropdownMenu.Trigger>
	<DropdownMenu.Content {side} {align} class="w-48">
		<AgentMoreMenuItems
			{agent}
			{running}
			{busy}
			onBusyChange={(next) => (busy = next)}
			includeChatActions
			includeChatLink
			includeServerControls
			includeWorkspaceLinks
			{onNavigate}
		/>
	</DropdownMenu.Content>
</DropdownMenu.Root>
