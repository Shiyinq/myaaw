<script lang="ts">
	import { agentSelection } from '$lib/stores';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import TopBarButton from '$lib/components/app/TopBarButton.svelte';
	import TerminalSession from '$lib/components/terminal/TerminalSession.svelte';
	import { RefreshCw } from 'lucide-svelte';

	const { t } = useTranslation();

	let sessionRef = $state<{ reconnect: () => Promise<void> } | null>(null);
	let phase = $state<'idle' | 'connecting' | 'connected' | 'disconnected' | 'error'>('idle');
	let enabled = $state<boolean | null>(null);
</script>

<TopBarActions id="terminal-actions">
	{#snippet children()}
		<TopBarButton
			label={t('myaaw.terminal.reconnect')}
			icon={RefreshCw}
			onclick={() => void sessionRef?.reconnect()}
			loading={phase === 'connecting'}
			disabled={enabled === false}
		/>
	{/snippet}
</TopBarActions>

<div class="flex h-full min-h-0 flex-col p-4 lg:p-6">
	{#if agentSelection.selected}
		<TerminalSession
			bind:this={sessionRef}
			agent={agentSelection.selected}
			onStateChange={(state) => {
				phase = state.phase;
				enabled = state.enabled;
			}}
		/>
	{/if}
</div>
