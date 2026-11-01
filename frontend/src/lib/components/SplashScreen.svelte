<script lang="ts">
	import Logo from '$lib/components/common/Logo.svelte';
</script>

<div class="fixed inset-0 z-[100] flex flex-col items-center justify-center gap-5 bg-background">
	<div
		class="flex h-16 w-16 items-center justify-center rounded-2xl bg-primary ring-8 ring-primary/5"
	>
		<Logo class="h-11 w-11 text-primary-foreground" />
	</div>
	<div class="flex flex-col items-center gap-1">
		<span class="text-2xl font-bold tracking-tight">Myaaw</span>
		<span class="text-[10px] font-medium uppercase tracking-[0.25em] text-muted-foreground">
			Control plane
		</span>
	</div>
</div>
