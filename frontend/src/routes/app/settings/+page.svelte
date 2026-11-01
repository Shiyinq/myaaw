<script lang="ts">
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { Button } from '$lib/components/ui/button';
	import {
		Card,
		CardContent,
		CardDescription,
		CardHeader,
		CardTitle
	} from '$lib/components/ui/card';
	import { Label } from '$lib/components/ui/label';
	import { Palette } from 'lucide-svelte';
	import { userPrefersMode, setMode, resetMode } from 'mode-watcher';
	import type { Locale } from '$lib/i18n';

	const { t, locale, changeLocale, availableLocales } = useTranslation();

	function changeLanguage(event: Event) {
		changeLocale((event.currentTarget as HTMLSelectElement).value as Locale);
	}
</script>

<div class="space-y-4">
	<Card>
		<CardHeader>
			<CardTitle class="flex items-center gap-2">
				<Palette class="h-4 w-4" />
				{t('myaaw.settings.app_settings')}
			</CardTitle>
			<CardDescription>{t('myaaw.settings.app_settings_desc')}</CardDescription>
		</CardHeader>
		<CardContent class="grid gap-4 sm:grid-cols-2">
			<div class="grid gap-1">
				<Label for="app-language">{t('myaaw.settings.language')}</Label>
				<select
					id="app-language"
					value={locale.value}
					onchange={changeLanguage}
					class="h-9 w-full rounded-md border border-input bg-transparent px-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
				>
					{#each availableLocales as item (item.code)}
						<option value={item.code}>{item.flag} {item.nativeName}</option>
					{/each}
				</select>
			</div>
			<div class="grid gap-1">
				<Label>{t('myaaw.settings.theme')}</Label>
				<div class="flex gap-2">
					<Button
						variant={userPrefersMode.current === 'light' ? 'default' : 'outline'}
						size="sm"
						class="flex-1"
						onclick={() => setMode('light')}
					>
						{t('myaaw.settings.theme_light')}
					</Button>
					<Button
						variant={userPrefersMode.current === 'dark' ? 'default' : 'outline'}
						size="sm"
						class="flex-1"
						onclick={() => setMode('dark')}
					>
						{t('myaaw.settings.theme_dark')}
					</Button>
					<Button
						variant={userPrefersMode.current === 'system' ? 'default' : 'outline'}
						size="sm"
						class="flex-1"
						onclick={() => resetMode()}
					>
						{t('myaaw.settings.theme_system')}
					</Button>
				</div>
			</div>
		</CardContent>
	</Card>
</div>
