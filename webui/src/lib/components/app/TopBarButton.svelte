<script lang="ts">
	import { Button } from '$lib/components/ui/button';

	interface Props {
		label: string;
		icon: any;
		onclick: () => void;
		loading?: boolean;
		disabled?: boolean;
		variant?: 'default' | 'outline' | 'secondary' | 'ghost' | 'destructive' | 'link';
	}

	let {
		label,
		icon: Icon,
		onclick,
		loading = false,
		disabled = false,
		variant = 'ghost'
	}: Props = $props();

	let tone = $derived(
		variant === 'ghost' ? 'text-muted-foreground hover:bg-muted/80 hover:text-foreground' : ''
	);
</script>

<Button
	{variant}
	size="sm"
	class={['hidden shrink-0 gap-1.5 rounded-full px-3 sm:inline-flex', tone]}
	{onclick}
	{loading}
	{disabled}
>
	<Icon class="h-3.5 w-3.5" />
	{label}
</Button>
<Button
	{variant}
	size="icon-sm"
	class={['shrink-0 rounded-full sm:hidden', tone]}
	{onclick}
	{loading}
	{disabled}
	aria-label={label}
>
	<Icon class="h-4 w-4" />
</Button>
