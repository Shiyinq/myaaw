<script lang="ts">
	let { label, variant = 'count' }: { label: string; variant?: 'count' | 'name' } = $props();
</script>

<div
	role="status"
	aria-label={label}
	class={['skeleton-group-delayed rounded bg-muted', variant === 'name' ? 'h-7 w-24' : 'h-8 w-12']}
></div>
