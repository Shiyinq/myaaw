<script lang="ts">
	import { fade } from 'svelte/transition';
</script>

<div class="loading-bar-container" out:fade={{ duration: 300 }}>
	<div class="loading-progress"></div>
</div>

<style>
	.loading-bar-container {
		position: fixed;
		top: 0;
		left: 0;
		width: 100%;
		height: 3px;
		z-index: 9999;
		background: rgba(0, 0, 0, 0.1);
	}

	.loading-progress {
		height: 100%;
		background: linear-gradient(to right, var(--primary), var(--secondary));
		width: 40%;
		animation: progress-move 2s infinite ease-in-out;
		box-shadow: 0 0 10px var(--primary-glow);
	}

	@keyframes progress-move {
		0% {
			margin-left: -40%;
		}
		100% {
			margin-left: 100%;
		}
	}
</style>
