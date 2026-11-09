<script lang="ts">
	interface Props {
		checked: boolean;
		/** Accessible name, e.g. "Enable or disable Weather". */
		label: string;
		disabled?: boolean;
		/** The switch sits inside clickable cards, so it stops the click it handles. */
		onchange: (checked: boolean) => void;
	}

	let { checked, label, disabled = false, onchange }: Props = $props();
</script>

<button
	type="button"
	role="switch"
	aria-checked={checked}
	aria-label={label}
	title={label}
	{disabled}
	onclick={(event) => {
		event.stopPropagation();
		onchange(!checked);
	}}
	class={[
		'relative inline-flex h-5 w-9 shrink-0 items-center rounded-full transition-colors',
		'focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:outline-none',
		disabled ? 'opacity-50' : 'cursor-pointer',
		checked ? 'bg-emerald-500' : 'bg-muted-foreground/30'
	]}
>
	<span
		class={[
			'h-4 w-4 rounded-full bg-white shadow-sm transition-transform',
			checked ? 'translate-x-4.5' : 'translate-x-0.5'
		]}
	></span>
</button>
