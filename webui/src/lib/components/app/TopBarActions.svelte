<script lang="ts">
	import { onMount } from 'svelte';
	import type { Snippet } from 'svelte';
	import { topBarSlot } from '$lib/stores';

	interface Props {
		id: string;
		children: Snippet;
	}

	let { id, children }: Props = $props();

	onMount(() => {
		topBarSlot.set({ id, content: children });
		return () => topBarSlot.clear(id);
	});
</script>
