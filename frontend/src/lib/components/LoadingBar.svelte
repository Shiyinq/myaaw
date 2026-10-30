<script lang="ts">
	import { navigating } from '$app/state';
	import { fade } from 'svelte/transition';

	let p = $state(0);
	let visible = $state(false);
	let interval: any;

	$effect(() => {
		// Di Svelte 5, navigating selalu berupa objek.
		// Kita harus cek properti .to untuk tahu apakah navigasi sedang aktif.
		if (navigating.to) {
			visible = true;
			p = 0.05;
			if (interval) clearInterval(interval);
			interval = setInterval(() => {
				if (p < 0.9) p += (0.9 - p) * 0.1;
			}, 150);
		} else {
			if (interval) clearInterval(interval);
			// Hanya jalankan finish jika p > 0 (artinya ada navigasi yang baru selesai)
			if (p > 0) {
				p = 1;
				const timeout = setTimeout(() => {
					visible = false;
					p = 0;
				}, 400);
				return () => clearTimeout(timeout);
			}
		}

		return () => {
			if (interval) clearInterval(interval);
		};
	});
</script>

{#if visible}
	<div
		transition:fade={{ duration: 200 }}
		class="fixed top-0 left-0 right-0 z-[1000000] pointer-events-none h-[3px] bg-primary/10"
	>
		<div
			class="h-full bg-primary shadow-[0_0_15px_var(--primary)] transition-all duration-300 ease-out"
			style="width: {p * 100}%;"
		></div>
	</div>
{/if}
