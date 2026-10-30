<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';

	onMount(() => {
		void goto('/app');
	});
</script>

<div class="min-h-screen bg-background" aria-hidden="true"></div>
