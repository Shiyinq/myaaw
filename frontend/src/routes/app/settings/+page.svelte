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
	import { Checkbox } from '$lib/components/ui/checkbox';
	import { Accessibility, Check, MessageSquare, Palette } from 'lucide-svelte';
	import { theme, userPrefersMode } from 'mode-watcher';
	import {
		applyAppearance,
		isAppearanceActive,
		MODES,
		THEMES,
		type AppearanceOption
	} from '$lib/theme';
	import { preferences } from '$lib/stores/preferences.svelte';
	import type { Locale } from '$lib/i18n';

	const { t, locale, changeLocale, availableLocales } = useTranslation();

	function changeLanguage(event: Event) {
		changeLocale((event.currentTarget as HTMLSelectElement).value as Locale);
	}

	const densityOptions = $derived([
		{ value: 'comfortable', label: t('myaaw.settings.density_comfortable') },
		{ value: 'compact', label: t('myaaw.settings.density_compact') }
	]);

	const sendOptions = $derived([
		{ value: 'enter', label: t('myaaw.settings.send_enter') },
		{ value: 'mod-enter', label: t('myaaw.settings.send_mod_enter') }
	]);

	const stepOptions = $derived([
		{ value: 'summary', label: t('myaaw.settings.steps_summary') },
		{ value: 'expanded', label: t('myaaw.settings.steps_expanded') }
	]);
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

{#snippet choice(
	options: { value: string; label: string }[],
	current: string,
	pick: (next: string) => void
)}
	<div class="inline-flex w-fit flex-wrap gap-0.5 rounded-lg border p-0.5">
		{#each options as option (option.value)}
			<button
				type="button"
				aria-pressed={current === option.value}
				onclick={() => pick(option.value)}
				class={[
					'rounded-md px-3 py-1.5 text-xs font-medium transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring/60',
					current === option.value
						? 'bg-primary text-primary-foreground'
						: 'text-muted-foreground hover:bg-muted hover:text-foreground'
				]}
			>
				{option.label}
			</button>
		{/each}
	</div>
{/snippet}

{#snippet toggle(
	label: string,
	description: string,
	checked: boolean,
	change: (next: boolean) => void
)}
	<label
		class="flex w-fit max-w-full cursor-pointer items-start gap-3 rounded-lg border border-border p-3 transition-colors hover:bg-muted/40"
	>
		<Checkbox {checked} onCheckedChange={(value) => change(value)} class="mt-0.5" />
		<span class="grid gap-0.5">
			<span class="text-sm font-medium">{label}</span>
			<span class="text-xs text-muted-foreground">{description}</span>
		</span>
	</label>
{/snippet}

<div class="mx-auto w-full max-w-4xl space-y-4">
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

	<Card>
		<CardHeader>
			<CardTitle class="flex items-center gap-2">
				<Accessibility class="h-4 w-4" />
				{t('myaaw.settings.accessibility')}
			</CardTitle>
			<CardDescription>{t('myaaw.settings.accessibility_desc')}</CardDescription>
		</CardHeader>
		<CardContent class="space-y-5">
			<div class="grid gap-2">
				<Label>{t('myaaw.settings.density')}</Label>
				{@render choice(densityOptions, preferences.value.density, (next) =>
					preferences.update({ density: next as 'comfortable' | 'compact' })
				)}
			</div>

			<div class="grid gap-2">
				<div class="flex items-center justify-between gap-3 sm:max-w-sm">
					<Label for="text-size">{t('myaaw.settings.text_size')}</Label>
					<span class="text-xs tabular-nums text-muted-foreground">
						{preferences.value.textScale}%
					</span>
				</div>
				<input
					id="text-size"
					type="range"
					min="100"
					max="125"
					step="5"
					value={preferences.value.textScale}
					oninput={(event) => preferences.update({ textScale: Number(event.currentTarget.value) })}
					class="h-1.5 w-full max-w-sm cursor-pointer accent-primary"
				/>
			</div>

			<div class="grid gap-2">
				{@render toggle(
					t('myaaw.settings.reduce_motion'),
					t('myaaw.settings.reduce_motion_desc'),
					preferences.value.reduceMotion,
					(next) => preferences.update({ reduceMotion: next })
				)}
			</div>
		</CardContent>
	</Card>

	<Card>
		<CardHeader>
			<CardTitle class="flex items-center gap-2">
				<MessageSquare class="h-4 w-4" />
				{t('myaaw.settings.chat')}
			</CardTitle>
			<CardDescription>{t('myaaw.settings.chat_desc')}</CardDescription>
		</CardHeader>
		<CardContent class="space-y-5">
			<div class="grid gap-2">
				<Label>{t('myaaw.settings.send_key')}</Label>
				{@render choice(sendOptions, preferences.value.sendKey, (next) =>
					preferences.update({ sendKey: next as 'enter' | 'mod-enter' })
				)}
			</div>

			<div class="grid gap-2">
				<Label>{t('myaaw.settings.steps')}</Label>
				{@render choice(stepOptions, preferences.value.steps, (next) =>
					preferences.update({ steps: next as 'summary' | 'expanded' })
				)}
			</div>

			<div class="flex flex-wrap gap-2">
				{@render toggle(
					t('myaaw.settings.auto_scroll'),
					t('myaaw.settings.auto_scroll_desc'),
					preferences.value.autoScroll,
					(next) => preferences.update({ autoScroll: next })
				)}
				{@render toggle(
					t('myaaw.settings.keep_drafts'),
					t('myaaw.settings.keep_drafts_desc'),
					preferences.value.keepDrafts,
					(next) => preferences.update({ keepDrafts: next })
				)}
			</div>
		</CardContent>
	</Card>
</div>
