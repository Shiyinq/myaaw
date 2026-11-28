<script lang="ts">
	import Bloub from '$lib/components/common/Bloub.svelte';
	import Logo from '$lib/components/common/Logo.svelte';
	import AgentField from '$lib/components/landing/AgentField.svelte';
</script>

<div
	class="fixed inset-0 z-100 flex flex-col items-center justify-center overflow-hidden bg-background"
>
	<div class="absolute inset-x-0 top-2 h-[calc(100svh+2.5rem)]">
		<AgentField class="mask-[radial-gradient(125%_115%_at_50%_46%,black_38%,transparent_94%)]" />
	</div>
	<div class="relative z-10 flex flex-col items-center justify-center gap-2">
		<Bloub cycle="idle2" size={112} eager tight>
			{#snippet fallback()}
				<Logo class="h-24 w-24" />
			{/snippet}
		</Bloub>
		<span class="text-2xl font-bold tracking-tight">Myaaw</span>
	</div>
</div>
