<script lang="ts">
	import { useTranslation } from '$lib/i18n/useTranslation';
	import {
		Card,
		CardContent,
		CardDescription,
		CardHeader,
		CardTitle
	} from '$lib/components/ui/card';
	import { Label } from '$lib/components/ui/label';
	import { Check, Palette } from 'lucide-svelte';
	import { theme, userPrefersMode } from 'mode-watcher';
	import {
		applyAppearance,
		isAppearanceActive,
		MODES,
		THEMES,
		type AppearanceOption
	} from '$lib/theme';
	import type { Locale } from '$lib/i18n';

	const { t, locale, changeLocale, availableLocales } = useTranslation();

	function changeLanguage(event: Event) {
		changeLocale((event.currentTarget as HTMLSelectElement).value as Locale);
	}
</script>

{#snippet themeTile(option: AppearanceOption)}
	{@const on = isAppearanceActive(option, userPrefersMode.current, theme.current)}
	<button
		type="button"
		aria-pressed={on}
		title={t(option.labelKey)}
		onclick={() => applyAppearance(option)}
		class={[
			'flex items-center gap-2.5 rounded-lg border p-2 text-left transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring/60',
			on ? 'border-primary bg-accent' : 'border-border hover:bg-muted'
		]}
	>
		<span
			class="relative flex h-8 w-11 shrink-0 items-center justify-center overflow-hidden rounded-md"
			style:background={option.swatch.bg}
		>
			<span
				class="h-3.5 w-5 rounded-sm ring-1 ring-neutral-400/40"
				style:background={option.swatch.card}
			></span>
			<span
				class="absolute right-1.5 bottom-1.5 h-1.5 w-1.5 rounded-full"
				style:background={option.swatch.accent}
			></span>
		</span>
		<span class="min-w-0 flex-1 truncate text-sm">{t(option.labelKey)}</span>
		{#if on}
			<Check class="h-3.5 w-3.5 shrink-0" />
		{/if}
	</button>
{/snippet}

<div class="mx-auto w-full max-w-4xl">
	<Card>
		<CardHeader>
			<CardTitle class="flex items-center gap-2">
				<Palette class="h-4 w-4" />
				{t('myaaw.settings.app_settings')}
			</CardTitle>
			<CardDescription>{t('myaaw.settings.app_settings_desc')}</CardDescription>
		</CardHeader>
		<CardContent class="space-y-6">
			<div class="grid gap-2 sm:max-w-xs">
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

			<div class="grid gap-2">
				<Label>{t('myaaw.settings.mode')}</Label>
				<div class="grid grid-cols-2 gap-2 sm:grid-cols-3">
					{#each MODES as option (option.id)}
						{@render themeTile(option)}
					{/each}
				</div>
			</div>

			<div class="grid gap-2">
				<Label>{t('myaaw.settings.theme')}</Label>
				<div class="grid grid-cols-2 gap-2 sm:grid-cols-3">
					{#each THEMES as option (option.id)}
						{@render themeTile(option)}
					{/each}
				</div>
			</div>
		</CardContent>
	</Card>
</div>
