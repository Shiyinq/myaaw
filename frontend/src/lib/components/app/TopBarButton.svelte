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
		variant = 'outline'
	}: Props = $props();
</script>

<Button {variant} size="sm" class="hidden shrink-0 sm:inline-flex" {onclick} {loading} {disabled}>
	<Icon class="mr-2 h-4 w-4" />
	{label}
</Button>
<Button
	{variant}
	size="icon-sm"
	class="shrink-0 sm:hidden"
	{onclick}
	{loading}
	{disabled}
	aria-label={label}
>
	<Icon class="h-4 w-4" />
</Button>
