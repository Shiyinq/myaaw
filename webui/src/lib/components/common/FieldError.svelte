<script lang="ts">
	import { useTranslation } from '$lib/i18n/useTranslation';

	interface Props {
		/** Message code emitted by the zod schemas, e.g. `required` or `url`. */
		code?: string;
	}

	let { code }: Props = $props();
	const { t } = useTranslation();
</script>

{#if code}
	<p class="text-[11px] text-destructive">{t(`myaaw.validation.${code}`)}</p>
{/if}
