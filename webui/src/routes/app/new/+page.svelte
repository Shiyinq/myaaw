<script lang="ts">
	import { page } from '$app/state';
	import AgentSetupForm from '$lib/components/onboard/AgentSetupForm.svelte';

	const agentName = $derived(page.url.searchParams.get('agent') ?? '');
	const initialRuntime = $derived(page.url.searchParams.get('runtime') ?? '');
</script>

<AgentSetupForm layout="page" {agentName} {initialRuntime} />
