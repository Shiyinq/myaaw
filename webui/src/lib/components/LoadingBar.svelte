<script lang="ts">
	import { navigating } from '$app/state';
	import { untrack } from 'svelte';
	import { fade } from 'svelte/transition';

	let p = $state(0);
	let visible = $state(false);

	$effect(() => {
		const shown = untrack(() => visible);
		if (!navigating.to) {
			if (!shown) return;
			p = 1;
			const hide = setTimeout(() => {
				visible = false;
			}, 400);
			return () => clearTimeout(hide);
		}

		let interval: ReturnType<typeof setInterval> | undefined;
		function start() {
			p = 0.05;
			visible = true;
			interval = setInterval(() => {
				if (p < 0.9) p += (0.9 - p) * 0.1;
			}, 150);
		}

		let delay: ReturnType<typeof setTimeout> | undefined;
		if (shown) start();
		else delay = setTimeout(start, 200);

		return () => {
			clearTimeout(delay);
			clearInterval(interval);
		};
	});
</script>

{#if visible}
	<div
		transition:fade={{ duration: 200 }}
		class="fixed top-0 left-0 right-0 z-1000000 pointer-events-none h-0.75 bg-primary/10"
	>
		<div
			class="h-full bg-primary shadow-[0_0_15px_var(--primary)] transition-all duration-300 ease-out"
			style="width: {p * 100}%;"
		></div>
	</div>
{/if}
