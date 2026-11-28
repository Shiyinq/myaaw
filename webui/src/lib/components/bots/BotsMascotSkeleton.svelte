<script lang="ts">
	import { useTranslation } from '$lib/i18n/useTranslation';

	const { t } = useTranslation();
</script>

<div
	aria-label={t('common.loading')}
	class="h-12 w-12 shrink-0 animate-pulse rounded-2xl bg-muted motion-reduce:animate-none"
></div>
