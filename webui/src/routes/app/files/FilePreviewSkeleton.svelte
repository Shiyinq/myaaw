<script lang="ts">
	import { Skeleton } from '$lib/components/ui/skeleton';

	let { kind }: { kind: 'image' | 'video' | 'audio' | 'pdf' } = $props();

	const mediaClasses = {
		image: 'h-40 w-40',
		video: 'aspect-video w-full max-w-2xl',
		audio: 'h-12 w-full max-w-lg'
	};
</script>

{#if kind === 'pdf'}
	<Skeleton class="h-full w-full" />
{:else}
	<div class="flex h-full items-center justify-center">
		<Skeleton class={mediaClasses[kind]} />
	</div>
{/if}
