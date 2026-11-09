<script lang="ts">
	import Bloub from '$lib/components/common/Bloub.svelte';
	import Logo from '$lib/components/common/Logo.svelte';
</script>

<div class="fixed inset-0 z-100 flex flex-col items-center justify-center gap-2 bg-background">
	<Bloub cycle="idle2" size={112} eager tight>
		{#snippet fallback()}
			<Logo class="h-24 w-24" />
		{/snippet}
	</Bloub>
	<div class="flex flex-col items-center gap-1">
		<span class="text-2xl font-bold tracking-tight">Myaaw</span>
		<span class="text-[10px] font-medium uppercase tracking-[0.25em] text-muted-foreground">
			Control plane
		</span>
	</div>
</div>
