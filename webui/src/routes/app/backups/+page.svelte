<script lang="ts">
	import { agentSelection } from '$lib/stores';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import BackupsPanel from '$lib/components/backup/BackupsPanel.svelte';
	import { Card, CardContent } from '$lib/components/ui/card';

	const { t } = useTranslation();

	// This page manages the selected agent only. Fleet-wide backup management
	// lives on the fleet page.
	let agent = $derived(agentSelection.selected);
</script>

{#if !agent}
	<Card>
		<CardContent class="py-10 text-center text-sm text-muted-foreground">
			{t('myaaw.agent.none')}
		</CardContent>
	</Card>
{:else}
	<BackupsPanel {agent} />
{/if}
