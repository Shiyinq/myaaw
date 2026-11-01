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
	import { Keyboard } from 'lucide-svelte';
	import { shortcutGroups } from '$lib/shortcuts';
	import { formatShortcut, isMac } from '$lib/utils/keys';
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

	<!-- The same list the help dialog shows, so it is discoverable without the `?` key. -->
	<Card>
		<CardHeader>
			<CardTitle class="flex items-center gap-2">
				<Keyboard class="h-4 w-4" />
				{t('myaaw.shortcuts.title')}
			</CardTitle>
			<CardDescription>{t('myaaw.shortcuts.settings_desc')}</CardDescription>
		</CardHeader>
		<CardContent class="space-y-4">
			{#each shortcutGroups as group (group.titleKey)}
				<section class="space-y-1.5">
					<p class="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
						{t(group.titleKey)}
					</p>
					{#each group.entries as entry (entry.combo)}
						<div class="flex items-center justify-between gap-4 text-sm">
							<span class="min-w-0 flex-1">{t(entry.labelKey)}</span>
							<span class="flex shrink-0 items-center gap-1.5">
								<kbd
									class="rounded border bg-muted px-1.5 py-0.5 font-mono text-[11px] text-muted-foreground"
								>
									{formatShortcut(entry.combo)}
								</kbd>
								{#if formatShortcut(entry.combo, isMac ? 'other' : 'mac') !== formatShortcut(entry.combo)}
									<span class="text-[10px] text-muted-foreground">
										{formatShortcut(entry.combo, isMac ? 'other' : 'mac')}
									</span>
								{/if}
							</span>
						</div>
					{/each}
				</section>
			{/each}
		</CardContent>
	</Card>
</div>
