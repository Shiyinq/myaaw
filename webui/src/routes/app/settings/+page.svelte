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
	import { Button } from '$lib/components/ui/button';
	import ConnectionPicker from '$lib/components/common/ConnectionPicker.svelte';
	import { Accessibility, Check, MessageSquare, Palette, Server } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { theme, userPrefersMode } from 'mode-watcher';
	import {
		applyAppearance,
		isAppearanceActive,
		MODES,
		THEMES,
		type AppearanceOption
	} from '$lib/theme';
	import { preferences } from '$lib/stores/preferences.svelte';
	import type { AudioLanguage, ShellMode, SpeechLanguage } from '$lib/stores/preferences.svelte';
	import { agentSelection, apiBase, connections } from '$lib/stores';
	import { tts } from '$lib/speech/tts.svelte';
	import { notificationSounds, playNotification } from '$lib/sound/notify.svelte';
	import {
		notificationStatus,
		previewBrowserNotification,
		requestNotificationPermission,
		type NotificationStatus
	} from '$lib/notifications/browser.svelte';
	import type { Locale } from '$lib/i18n';
	import type { PageData } from './$types';

	const { t, locale, changeLocale, availableLocales } = useTranslation();

	let { data }: { data: PageData } = $props();

	function changeLanguage(event: Event) {
		changeLocale((event.currentTarget as HTMLSelectElement).value as Locale);
	}

	const densityOptions = $derived([
		{ value: 'comfortable', label: t('myaaw.settings.density_comfortable') },
		{ value: 'compact', label: t('myaaw.settings.density_compact') }
	]);

	const shellModeOptions = $derived([
		{ value: 'advanced', label: t('myaaw.settings.mode_advanced') },
		{ value: 'classic', label: t('myaaw.settings.mode_classic') },
		{ value: 'focus', label: t('myaaw.settings.mode_focus') }
	]);

	/** Switching to focus mode needs an agent; default to the current one. */
	function pickShellMode(next: string) {
		const mode = next as ShellMode;
		const focusAgent =
			preferences.value.focusAgent || agentSelection.current?.name || agentSelection.selected || '';
		preferences.update({ shellMode: mode, focusAgent });
	}

	const sendOptions = $derived([
		{ value: 'enter', label: t('myaaw.settings.send_enter') },
		{ value: 'mod-enter', label: t('myaaw.settings.send_mod_enter') }
	]);

	const stepOptions = $derived([
		{ value: 'summary', label: t('myaaw.settings.steps_summary') },
		{ value: 'expanded', label: t('myaaw.settings.steps_expanded') }
	]);

	// The synthesiser list is empty until the browser announces it, so the card
	// waits for the mount before claiming support.
	let speechSupported = $state(false);

	// WebUI connection. Empty means the same-origin proxy; an absolute URL makes
	// the browser talk to a remote control plane (tunnel/SSH).
	let serverUrl = $state('');

	onMount(() => {
		speechSupported = tts.supported;
		tts.refreshVoices();
		connections.seedFromServer(data.apiUrl ?? '', data.binaryInstalled ?? false);
		serverUrl = apiBase.isCustom ? apiBase.value : '';
	});

	function applyServerUrl() {
		apiBase.set(serverUrl);
		if (typeof window !== 'undefined') window.location.reload();
	}

	const speechLanguageOptions = $derived([
		{ value: 'auto', label: t('myaaw.settings.speech_language_auto') },
		{ value: 'app', label: t('myaaw.settings.speech_language_app') },
		{ value: 'id', label: t('myaaw.settings.speech_language_id') },
		{ value: 'en', label: t('myaaw.settings.speech_language_en') }
	]);

	const dictationLanguageOptions = $derived([
		{ value: 'app', label: t('myaaw.settings.speech_language_app') },
		{ value: 'id', label: t('myaaw.settings.speech_language_id') },
		{ value: 'en', label: t('myaaw.settings.speech_language_en') }
	]);

	const languageFilter = $derived.by(() => {
		const choice = preferences.value.speechLang;
		if (choice === 'auto') return '';
		return choice === 'app' ? locale.value : choice;
	});

	const voiceOptions = $derived(
		tts.voices
			.filter((voice) => !languageFilter || voice.lang.toLowerCase().startsWith(languageFilter))
			.sort((a, b) => a.name.localeCompare(b.name))
	);

	const speechRate = $derived(preferences.value.speechRate.toFixed(2));
	const notifyVolumePercent = $derived(Math.round(preferences.value.notifyVolume * 100));

	// The browser permission is not reactive, so the page caches it and refreshes
	// it when the reader comes back from the browser's site settings.
	let browserNotifyStatus = $state<NotificationStatus>(notificationStatus());

	$effect(() => {
		if (typeof window === 'undefined') return;
		const sync = () => (browserNotifyStatus = notificationStatus());
		window.addEventListener('focus', sync);
		return () => window.removeEventListener('focus', sync);
	});

	async function enableBrowserNotifications() {
		browserNotifyStatus = notificationStatus();
		if (browserNotifyStatus === 'granted') {
			preferences.update({ notifyBrowser: true });
			return;
		}
		browserNotifyStatus = await requestNotificationPermission();
		preferences.update({ notifyBrowser: browserNotifyStatus === 'granted' });
	}

	// The Notification API has no delivery receipt, so a successful call only
	// means the browser accepted it; the hint points at the OS layer that can
	// still swallow it.
	let browserTestSent = $state(false);

	function testBrowserNotification() {
		browserTestSent = previewBrowserNotification();
	}

	function previewSpeech() {
		tts.speak('settings-preview', t('myaaw.settings.speech_sample'));
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
				<Server class="h-4 w-4" />
				{t('myaaw.settings.connection')}
			</CardTitle>
			<CardDescription>{t('myaaw.settings.connection_desc')}</CardDescription>
		</CardHeader>
		<CardContent class="space-y-4">
			<div class="grid gap-2 sm:max-w-md">
				<Label for="server-url">{t('myaaw.settings.server_url')}</Label>
				<ConnectionPicker bind:value={serverUrl} inputId="server-url" />
			</div>
			<Button variant="outline" size="sm" class="w-fit" onclick={applyServerUrl}>
				{t('myaaw.settings.save_reconnect')}
			</Button>
			<p class="text-xs text-muted-foreground">
				{#if apiBase.isCustom}
					{t('myaaw.settings.connection_remote', { url: apiBase.value })}
				{:else}
					{t('myaaw.settings.connection_local')}
				{/if}
			</p>
		</CardContent>
	</Card>
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

			<div class="grid gap-2 border-t pt-5">
				<div class="grid gap-1">
					<Label>{t('myaaw.settings.shell_mode')}</Label>
					<p class="text-xs text-muted-foreground">{t('myaaw.settings.shell_mode_desc')}</p>
				</div>
				{@render choice(shellModeOptions, preferences.value.shellMode, pickShellMode)}
				{#if preferences.value.shellMode === 'focus'}
					<div class="mt-1 grid max-w-sm gap-2 rounded-lg border border-dashed p-3">
						<div class="grid gap-1">
							<Label for="focus-agent">{t('myaaw.settings.focus_agent')}</Label>
							<p class="text-xs text-muted-foreground">
								{t('myaaw.settings.focus_agent_desc')}
							</p>
						</div>
						{#if agentSelection.agents.length > 0}
							<select
								id="focus-agent"
								class="h-9 w-full rounded-md border border-input bg-transparent px-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
								value={preferences.value.focusAgent || agentSelection.current?.name || ''}
								onchange={(event) => preferences.update({ focusAgent: event.currentTarget.value })}
							>
								{#each agentSelection.agents as item (item.name)}
									<option value={item.name}>
										{item.name}{item.default ? ` · ${t('myaaw.agent.default')}` : ''}
									</option>
								{/each}
							</select>
						{:else}
							<p class="text-xs text-muted-foreground">{t('myaaw.agent.none')}</p>
						{/if}
					</div>
				{/if}
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

			<div class="grid gap-2">
				{@render toggle(
					t('myaaw.settings.host_badge'),
					t('myaaw.settings.host_badge_desc'),
					preferences.value.showHostBadge,
					(next) => preferences.update({ showHostBadge: next })
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

			<div class="grid gap-5 border-t pt-5">
				{#if speechSupported}
					<div class="grid gap-2">
						{@render toggle(
							t('myaaw.settings.speak_responses'),
							t('myaaw.settings.speak_responses_desc'),
							preferences.value.speakResponses,
							(next) => preferences.update({ speakResponses: next })
						)}
					</div>

					<div class="grid gap-2 sm:max-w-xs">
						<Label for="speech-language">{t('myaaw.settings.speech_language')}</Label>
						<select
							id="speech-language"
							value={preferences.value.speechLang}
							onchange={(event) =>
								preferences.update({
									speechLang: event.currentTarget.value as SpeechLanguage
								})}
							class="h-9 w-full rounded-md border border-input bg-transparent px-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
						>
							{#each speechLanguageOptions as option (option.value)}
								<option value={option.value}>{option.label}</option>
							{/each}
						</select>
					</div>

					<div class="grid gap-2 sm:max-w-xs">
						<Label for="speech-voice">{t('myaaw.settings.speech_voice')}</Label>
						<select
							id="speech-voice"
							value={preferences.value.speechVoice}
							onfocus={() => tts.refreshVoices()}
							onchange={(event) => preferences.update({ speechVoice: event.currentTarget.value })}
							class="h-9 w-full rounded-md border border-input bg-transparent px-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
						>
							<option value="">{t('myaaw.settings.speech_voice_default')}</option>
							{#each voiceOptions as voice (voice.voiceURI)}
								<option value={voice.voiceURI}>
									{voice.name} · {voice.lang}{voice.localService
										? ''
										: ` · ${t('myaaw.settings.speech_voice_online')}`}
								</option>
							{/each}
						</select>
						{#if tts.voices.length === 0}
							<p class="text-xs text-muted-foreground">
								{t('myaaw.settings.speech_loading_voices')}
							</p>
						{:else if languageFilter && voiceOptions.length === 0}
							<p class="text-xs text-muted-foreground">
								{t('myaaw.settings.speech_no_voice_hint')}
							</p>
						{/if}
					</div>

					<div class="grid gap-2">
						<div class="flex items-center justify-between gap-3 sm:max-w-sm">
							<Label for="speech-rate">{t('myaaw.settings.speech_rate')}</Label>
							<span class="text-xs tabular-nums text-muted-foreground">{speechRate}×</span>
						</div>
						<input
							id="speech-rate"
							type="range"
							min="0.75"
							max="1.5"
							step="0.05"
							value={preferences.value.speechRate}
							oninput={(event) =>
								preferences.update({ speechRate: Number(event.currentTarget.value) })}
							class="h-1.5 w-full max-w-sm cursor-pointer accent-primary"
						/>
						<div>
							<Button variant="outline" size="sm" onclick={previewSpeech}>
								{t('myaaw.settings.speech_preview')}
							</Button>
						</div>
					</div>

					<p class="text-xs text-muted-foreground">{t('myaaw.settings.speech_note')}</p>
				{:else}
					<p class="text-sm text-muted-foreground">
						{t('myaaw.settings.speech_unsupported_note')}
					</p>
				{/if}

				<div class="grid gap-2 sm:max-w-xs">
					<Label for="dictation-language">{t('myaaw.settings.dictation_language')}</Label>
					<select
						id="dictation-language"
						value={preferences.value.dictationLang}
						onchange={(event) =>
							preferences.update({
								dictationLang: event.currentTarget.value as AudioLanguage
							})}
						class="h-9 w-full rounded-md border border-input bg-transparent px-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
					>
						{#each dictationLanguageOptions as option (option.value)}
							<option value={option.value}>{option.label}</option>
						{/each}
					</select>
					<p class="text-xs text-muted-foreground">
						{t('myaaw.settings.dictation_language_desc')}
					</p>
				</div>

				<div class="grid gap-3 border-t pt-5">
					{@render toggle(
						t('myaaw.settings.notify_responses'),
						t('myaaw.settings.notify_responses_desc'),
						preferences.value.notifyEnabled,
						(next) => preferences.update({ notifyEnabled: next })
					)}

					<div class="grid gap-2 sm:max-w-xs">
						<Label for="notify-sound">{t('myaaw.settings.notify_sound')}</Label>
						<select
							id="notify-sound"
							value={preferences.value.notifySound}
							onchange={(event) => preferences.update({ notifySound: event.currentTarget.value })}
							class="h-9 w-full rounded-md border border-input bg-transparent px-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
						>
							{#each notificationSounds as sound (sound.id)}
								<option value={sound.id}>{t(sound.labelKey)}</option>
							{/each}
						</select>

						<div class="grid gap-2">
							<div class="flex items-center justify-between gap-3">
								<Label for="notify-volume">{t('myaaw.settings.notify_volume')}</Label>
								<span class="text-xs tabular-nums text-muted-foreground"
									>{notifyVolumePercent}%</span
								>
							</div>
							<input
								id="notify-volume"
								type="range"
								min="0"
								max="1"
								step="0.05"
								value={preferences.value.notifyVolume}
								oninput={(event) =>
									preferences.update({ notifyVolume: Number(event.currentTarget.value) })}
								class="h-1.5 w-full cursor-pointer accent-primary"
							/>
						</div>

						<div>
							<Button
								variant="outline"
								size="sm"
								onclick={() =>
									playNotification(preferences.value.notifySound, preferences.value.notifyVolume)}
							>
								{t('myaaw.settings.notify_preview')}
							</Button>
						</div>
					</div>

					<div class="grid gap-3 border-t pt-5">
						{#if browserNotifyStatus === 'granted'}
							{@render toggle(
								t('myaaw.settings.notify_browser'),
								t('myaaw.settings.notify_browser_desc'),
								preferences.value.notifyBrowser,
								(next) => preferences.update({ notifyBrowser: next })
							)}
							<div class="grid gap-2">
								<Button variant="outline" size="sm" class="w-fit" onclick={testBrowserNotification}>
									{t('myaaw.settings.notify_browser_test')}
								</Button>
								{#if browserTestSent}
									<p class="max-w-md text-xs text-muted-foreground">
										{t('myaaw.settings.notify_browser_test_sent')}
									</p>
								{/if}
							</div>
						{:else}
							<div class="grid w-fit max-w-full gap-2 rounded-lg border border-border p-3">
								<span class="grid gap-0.5">
									<span class="text-sm font-medium">{t('myaaw.settings.notify_browser')}</span>
									<span class="text-xs text-muted-foreground"
										>{t('myaaw.settings.notify_browser_desc')}</span
									>
									{#if browserNotifyStatus === 'denied'}
										<span class="text-xs text-muted-foreground"
											>{t('myaaw.settings.notify_browser_denied')}</span
										>
									{:else if browserNotifyStatus === 'unsupported'}
										<span class="text-xs text-muted-foreground"
											>{t('myaaw.settings.notify_browser_unsupported')}</span
										>
									{/if}
								</span>
								<Button
									variant="outline"
									size="sm"
									class="w-fit"
									disabled={browserNotifyStatus === 'unsupported'}
									onclick={() => void enableBrowserNotifications()}
								>
									{t('myaaw.settings.notify_browser_enable')}
								</Button>
							</div>
						{/if}
					</div>
				</div>
			</div></CardContent
		>
	</Card>
</div>
