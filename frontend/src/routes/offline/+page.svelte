<script lang="ts">
	import { WifiOff } from 'lucide-svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { Button } from '$lib/components/ui/button';

	const { t } = useTranslation();
</script>

<div class="min-h-screen flex flex-col items-center justify-center bg-background px-4">
	<div class="text-center max-w-md w-full">
		<WifiOff class="mx-auto h-24 w-24 text-muted-foreground mb-6" />
		<h1 class="text-3xl font-bold text-foreground mb-4">{t('offline.title')}</h1>
		<p class="text-muted-foreground mx-auto mb-8">
			{t('offline.description')}
		</p>
		<Button size="lg" class="w-full sm:w-auto font-medium" onclick={() => window.location.reload()}>
			{t('offline.reloadButton')}
		</Button>
	</div>
</div>
