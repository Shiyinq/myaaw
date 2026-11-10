<script lang="ts">
	import Bloub from '$lib/components/common/Bloub.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { Button } from '$lib/components/ui/button';

	const { t } = useTranslation();
</script>

<div class="min-h-screen flex flex-col items-center justify-center bg-background page-clear px-4">
	<div class="text-center max-w-md w-full">
		<Bloub cycle="sleep" size={96} tight class="mx-auto block" />
		<h1 class="mt-3 mb-4 text-3xl font-bold text-foreground">{t('offline.title')}</h1>
		<p class="text-muted-foreground mx-auto mb-8">
			{t('offline.description')}
		</p>
		<Button size="lg" class="w-full sm:w-auto font-medium" onclick={() => window.location.reload()}>
			{t('offline.reloadButton')}
		</Button>
	</div>
</div>
