<script lang="ts">
	import Bloub from '$lib/components/common/Bloub.svelte';
	import { BLOUB_COLORS, BLOUB_DEFAULT_COLOR, type BloubColor } from '$lib/bloub';
	import { useTranslation } from '$lib/i18n/useTranslation';

	interface Props {
		value?: BloubColor;
		/** Radio group name; only needed when two pickers share a page. */
		name?: string;
		disabled?: boolean;
	}

	let {
		value = $bindable(BLOUB_DEFAULT_COLOR),
		name = 'agent-color',
		disabled = false
	}: Props = $props();

	const { t } = useTranslation();
</script>

<!--
	Every option is the attentive face in a different palette colour: the agent
	keeps one expression and only its ink changes. Real radios rather than
	buttons, so arrow keys work and the choice is announced as a choice.

	The swatch sits on a bordered chip because `creme` (#f1efe9) and `gris` would
	otherwise disappear into the card.
-->
<div
	role="radiogroup"
	aria-label={t('myaaw.onboard.agent_color_title')}
	class="grid grid-cols-6 gap-2 sm:grid-cols-12"
>
	{#each BLOUB_COLORS as color (color)}
		<label class="block cursor-pointer">
			<input
				class="peer sr-only"
				type="radio"
				{name}
				value={color}
				bind:group={value}
				{disabled}
				aria-label={t(`myaaw.bloub.color.${color}`)}
			/>
			<span
				class="flex aspect-square items-center justify-center rounded-xl border bg-background transition-colors hover:bg-muted peer-checked:border-primary peer-checked:bg-accent peer-focus-visible:ring-2 peer-focus-visible:ring-ring/50 peer-disabled:cursor-not-allowed peer-disabled:opacity-50"
			>
				<Bloub mood="attentif" size={34} {color} />
			</span>
		</label>
	{/each}
</div>
