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
	import SidebarHeader from '$lib/components/app/SidebarHeader.svelte';
	import {
		Accessibility,
		Bell,
		Check,
		MessageSquare,
		Palette,
		Server,
		Volume2
	} from 'lucide-svelte';
	import { onMount, tick } from 'svelte';
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
	import { revealPane } from '$lib/utils/scroll';
	import type { Locale } from '$lib/i18n';
	import type { PageData } from './$types';

	const { t, locale, changeLocale, availableLocales } = useTranslation();

	let { data }: { data: PageData } = $props();

	type SettingsGroupId =
		| 'appearance'
		| 'chat'
		| 'voice'
		| 'notifications'
		| 'accessibility'
		| 'connection';

	const VALID_GROUPS: readonly SettingsGroupId[] = [
		'appearance',
		'chat',
		'voice',
		'notifications',
		'accessibility',
		'connection'
	];

	let activeGroup = $state<SettingsGroupId>('appearance');
	let detail = $state<HTMLElement | null>(null);

	const groups = $derived([
		{
			id: 'appearance' as const,
			label: t('myaaw.settings.appearance'),
			description: t('myaaw.settings.appearance_desc'),
			icon: Palette
		},
		{
			id: 'chat' as const,
			label: t('myaaw.settings.chat'),
			description: t('myaaw.settings.chat_desc'),
			icon: MessageSquare
		},
		{
			id: 'voice' as const,
			label: t('myaaw.settings.voice'),
			description: t('myaaw.settings.voice_desc'),
			icon: Volume2
		},
		{
			id: 'notifications' as const,
			label: t('myaaw.settings.notifications'),
			description: t('myaaw.settings.notifications_desc'),
			icon: Bell
		},
		{
			id: 'accessibility' as const,
			label: t('myaaw.settings.accessibility'),
			description: t('myaaw.settings.accessibility_desc'),
			icon: Accessibility
		},
		{
			id: 'connection' as const,
			label: t('myaaw.settings.connection'),
			description: t('myaaw.settings.connection_desc'),
			icon: Server
		}
	]);

	function isGroupId(value: string): value is SettingsGroupId {
		return (VALID_GROUPS as readonly string[]).includes(value);
	}

	async function pickGroup(id: SettingsGroupId) {
		activeGroup = id;
		if (typeof window !== 'undefined') {
			const nextUrl = `${window.location.pathname}${window.location.search}#${id}`;
			window.history.replaceState(window.history.state, '', nextUrl);
		}
		await tick();
		if (detail) {
			detail.scrollTop = 0;
			revealPane(detail);
		}
	}

	function changeLanguage(event: Event) {
		changeLocale((event.currentTarget as HTMLSelectElement).value as Locale);
	}

	const densityOptions = $derived([
		{ value: 'comfortable', label: t('myaaw.settings.density_comfortable') },
		{ value: 'compact', label: t('myaaw.settings.density_compact') }
	]);

	const shellModeOptions = $derived<{ value: ShellMode; label: string; description: string }[]>([
		{
			value: 'bots',
			label: t('myaaw.settings.mode_bots'),
			description: t('myaaw.settings.mode_bots_desc')
		},
		{
			value: 'studio',
			label: t('myaaw.settings.mode_studio'),
			description: t('myaaw.settings.mode_studio_desc')
		},
		{
			value: 'focus',
			label: t('myaaw.settings.mode_focus'),
			description: t('myaaw.settings.mode_focus_desc')
		},
		{
			value: 'classic',
			label: t('myaaw.settings.mode_classic'),
			description: t('myaaw.settings.mode_classic_desc')
		}
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

	// The two send-while-streaming modes are mutually exclusive: turning one on
	// switches the other off, and both may be off at once.
	function setQueueMessages(next: boolean) {
		preferences.update({ queueMessages: next, ...(next ? { interruptOnSend: false } : {}) });
	}

	function setInterruptOnSend(next: boolean) {
		preferences.update({ interruptOnSend: next, ...(next ? { queueMessages: false } : {}) });
	}

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

		const hash = window.location.hash.replace(/^#/, '');
		if (isGroupId(hash)) {
			activeGroup = hash;
		}
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
	<div class="inline-flex w-fit max-w-full flex-wrap gap-0.5 rounded-lg border p-0.5">
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

{#snippet shellPreview(mode: ShellMode, active: boolean)}
	<div
		aria-hidden="true"
		class={[
			'relative flex h-24 w-full overflow-hidden rounded-lg border bg-background transition-colors',
			active ? 'border-primary/40' : 'border-border/80'
		]}
	>
		{#if mode === 'bots'}
			<!-- Left bot roster sidebar -->
			<div class="flex w-[28%] shrink-0 flex-col gap-1.5 border-r border-border/70 bg-card p-2">
				<div class="h-1.5 w-8 rounded-full bg-muted-foreground/25"></div>
				<div class="flex items-center gap-1.5 rounded-md bg-primary/15 p-1">
					<span class="h-2.5 w-2.5 shrink-0 rounded-full bg-primary"></span>
					<span class="h-1.5 flex-1 rounded-full bg-primary/50"></span>
				</div>
				<div class="flex items-center gap-1.5 px-1">
					<span class="h-2.5 w-2.5 shrink-0 rounded-full bg-muted-foreground/30"></span>
					<span class="h-1.5 flex-1 rounded-full bg-muted-foreground/20"></span>
				</div>
				<div class="flex items-center gap-1.5 px-1">
					<span class="h-2.5 w-2.5 shrink-0 rounded-full bg-muted-foreground/25"></span>
					<span class="h-1.5 w-2/3 rounded-full bg-muted-foreground/15"></span>
				</div>
			</div>
			<!-- Center chat -->
			<div class="flex min-w-0 flex-1 flex-col justify-between p-2">
				<div class="space-y-1.5">
					<div class="h-2.5 w-3/4 rounded-md bg-muted"></div>
					<div class="ml-auto h-2.5 w-2/3 rounded-md bg-primary/75"></div>
					<div class="h-2.5 w-4/5 rounded-md bg-muted"></div>
				</div>
				<div class="h-3 w-full rounded-full border border-border/80 bg-card"></div>
			</div>
			<!-- Right agent inspector panel -->
			<div class="flex w-[22%] shrink-0 flex-col gap-1.5 border-l border-border/70 bg-card/80 p-2">
				<div class="h-1.5 w-full rounded-full bg-muted-foreground/25"></div>
				<div class="h-4 w-full rounded bg-muted/80"></div>
				<div class="h-1.5 w-4/5 rounded-full bg-muted-foreground/15"></div>
				<div class="h-1.5 w-3/5 rounded-full bg-muted-foreground/15"></div>
			</div>
		{:else if mode === 'studio'}
			<!-- Left full navigation sidebar -->
			<div class="flex w-[26%] shrink-0 flex-col gap-1.5 border-r border-border/70 bg-card p-2">
				<div class="h-2.5 w-full rounded bg-primary/15"></div>
				<div class="mt-0.5 h-1 w-6 rounded-full bg-muted-foreground/25"></div>
				<div class="h-1.5 w-4/5 rounded-full bg-muted-foreground/30"></div>
				<div class="h-1.5 w-full rounded-full bg-muted-foreground/20"></div>
				<div class="mt-0.5 h-1 w-7 rounded-full bg-muted-foreground/25"></div>
				<div class="h-1.5 w-3/4 rounded-full bg-muted-foreground/20"></div>
				<div class="h-1.5 w-5/6 rounded-full bg-muted-foreground/20"></div>
			</div>
			<!-- Full dashboard workspace -->
			<div class="flex min-w-0 flex-1 flex-col gap-2 p-2">
				<div class="flex items-center justify-between">
					<div class="h-2 w-12 rounded-full bg-muted-foreground/30"></div>
					<div class="h-2 w-6 rounded-full bg-muted"></div>
				</div>
				<div class="grid flex-1 grid-cols-2 gap-1.5">
					<div class="rounded border border-border/70 bg-card p-1.5">
						<div class="h-1.5 w-8 rounded-full bg-primary/40"></div>
						<div class="mt-1.5 h-1 w-full rounded-full bg-muted"></div>
					</div>
					<div class="rounded border border-border/70 bg-card p-1.5">
						<div class="h-1.5 w-6 rounded-full bg-muted-foreground/25"></div>
						<div class="mt-1.5 h-1 w-4/5 rounded-full bg-muted"></div>
					</div>
					<div class="col-span-2 rounded border border-border/70 bg-card p-1.5">
						<div class="h-1.5 w-14 rounded-full bg-muted-foreground/25"></div>
						<div class="mt-1.5 h-1 w-3/4 rounded-full bg-muted"></div>
					</div>
				</div>
			</div>
		{:else if mode === 'focus'}
			<!-- Minimal left icon rail -->
			<div
				class="flex w-7 shrink-0 flex-col items-center gap-2 border-r border-border/70 bg-card py-2.5"
			>
				<span class="h-2.5 w-2.5 rounded-full bg-primary"></span>
				<span class="h-1.5 w-1.5 rounded-full bg-muted-foreground/30"></span>
				<span class="h-1.5 w-1.5 rounded-full bg-muted-foreground/30"></span>
				<span class="mt-auto h-1.5 w-1.5 rounded-full bg-muted-foreground/25"></span>
			</div>
			<!-- Centered single-agent chat -->
			<div class="flex min-w-0 flex-1 flex-col items-center justify-between px-4 py-2.5">
				<div class="w-full max-w-[78%] space-y-1.5">
					<div class="h-2.5 w-3/4 rounded-md bg-muted"></div>
					<div class="ml-auto h-2.5 w-1/2 rounded-md bg-primary/75"></div>
					<div class="h-2.5 w-5/6 rounded-md bg-muted"></div>
				</div>
				<div class="h-3.5 w-full max-w-[82%] rounded-full border border-border/80 bg-card"></div>
			</div>
		{:else}
			<!-- Classic: agent picker + chat history sidebar -->
			<div class="flex w-[28%] shrink-0 flex-col gap-1.5 border-r border-border/70 bg-card p-2">
				<div class="h-3 w-full rounded border border-border/80 bg-background"></div>
				<div class="mt-0.5 h-2 w-full rounded bg-primary/15"></div>
				<div class="h-1.5 w-4/5 rounded-full bg-muted-foreground/25"></div>
				<div class="h-1.5 w-full rounded-full bg-muted-foreground/20"></div>
				<div class="h-1.5 w-2/3 rounded-full bg-muted-foreground/15"></div>
			</div>
			<!-- Wide chat area -->
			<div class="flex min-w-0 flex-1 flex-col justify-between p-2.5">
				<div class="space-y-1.5">
					<div class="h-2.5 w-2/3 rounded-md bg-muted"></div>
					<div class="ml-auto h-2.5 w-1/2 rounded-md bg-primary/75"></div>
					<div class="h-2.5 w-3/4 rounded-md bg-muted"></div>
				</div>
				<div class="h-3.5 w-full rounded-full border border-border/80 bg-card"></div>
			</div>
		{/if}
	</div>
{/snippet}

{#snippet shellTile(option: { value: ShellMode; label: string; description: string })}
	{@const active = preferences.value.shellMode === option.value}
	<div
		class={[
			'group flex flex-col gap-2.5 rounded-xl border p-2.5 transition-colors',
			active
				? 'border-primary bg-accent/60 ring-1 ring-primary/20'
				: 'border-border hover:border-border/80 hover:bg-muted/40'
		]}
	>
		<button
			type="button"
			data-shell-mode={option.value}
			aria-pressed={active}
			onclick={() => pickShellMode(option.value)}
			class="flex w-full flex-col gap-2.5 rounded-lg text-left outline-none focus-visible:ring-2 focus-visible:ring-ring/60"
		>
			{@render shellPreview(option.value, active)}
			<div class="flex items-start justify-between gap-2 px-0.5">
				<div class="min-w-0 flex-1">
					<span class="block text-sm font-medium text-foreground">{option.label}</span>
					<span class="mt-0.5 block text-xs leading-snug text-muted-foreground">
						{option.description}
					</span>
				</div>
				{#if active}
					<span
						class="mt-0.5 flex h-4 w-4 shrink-0 items-center justify-center rounded-full bg-primary text-primary-foreground"
					>
						<Check class="h-2.5 w-2.5" />
					</span>
				{/if}
			</div>
		</button>
		{#if option.value === 'focus' && active}
			<div class="grid gap-1.5 border-t border-border/70 px-0.5 pt-2.5">
				<div class="grid gap-0.5">
					<Label for="focus-agent" class="text-xs">{t('myaaw.settings.focus_agent')}</Label>
					<p class="text-[11px] leading-snug text-muted-foreground">
						{t('myaaw.settings.focus_agent_desc')}
					</p>
				</div>
				{#if agentSelection.agents.length > 0}
					<select
						id="focus-agent"
						class="h-8 w-full rounded-md border border-input bg-background px-2 text-xs outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
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
		<span class="grid min-w-0 gap-0.5">
			<span class="text-sm font-medium">{label}</span>
			<span class="text-xs text-muted-foreground">{description}</span>
		</span>
	</label>
{/snippet}

<div class="flex h-full min-h-0 flex-col overflow-y-auto lg:flex-row lg:overflow-hidden">
	<!-- Setting groups: a second sidebar nested inside the main shell sidebar. -->
	<aside
		class="flex shrink-0 flex-col border-b bg-card frost-aside lg:h-full lg:w-64 lg:border-r lg:border-b-0 lg:pt-14 xl:w-72"
	>
		<div class="shrink-0 px-4 py-2.5 lg:pt-1">
			<SidebarHeader label={t('myaaw.settings.groups')} count={groups.length} />
		</div>
		<nav
			aria-label={t('myaaw.settings.title')}
			class="space-y-0.5 p-2 lg:min-h-0 lg:flex-1 lg:overflow-y-auto"
		>
			{#each groups as group (group.id)}
				{@const Icon = group.icon}
				{@const active = activeGroup === group.id}
				<button
					type="button"
					data-settings-group={group.id}
					aria-current={active ? 'page' : undefined}
					onclick={() => void pickGroup(group.id)}
					class={[
						'flex w-full items-start gap-2.5 rounded-lg px-2.5 py-2 text-left transition-colors list-row outline-none focus-visible:ring-2 focus-visible:ring-ring/60',
						active
							? 'bg-accent font-medium text-accent-foreground'
							: 'text-foreground/85 hover:bg-muted'
					]}
				>
					<Icon
						class={`mt-0.5 h-4 w-4 shrink-0 ${
							active ? 'text-accent-foreground' : 'text-muted-foreground'
						}`}
					/>
					<span class="min-w-0 flex-1">
						<span class="block truncate text-sm leading-tight">{group.label}</span>
						<span class="mt-0.5 block truncate text-[11px] font-normal text-muted-foreground">
							{group.description}
						</span>
					</span>
				</button>
			{/each}
		</nav>
	</aside>

	<section
		bind:this={detail}
		class="min-w-0 flex-1 p-4 lg:mt-14 lg:min-h-0 lg:overflow-y-auto lg:px-8 lg:pt-2 lg:pb-8"
	>
		<div class="mx-auto w-full max-w-3xl">
			{#if activeGroup === 'appearance'}
				<Card>
					<CardHeader>
						<CardTitle class="flex items-center gap-2">
							<Palette class="h-4 w-4" />
							{t('myaaw.settings.appearance')}
						</CardTitle>
						<CardDescription>{t('myaaw.settings.appearance_desc')}</CardDescription>
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

						<div class="grid gap-3 border-t pt-5">
							<div class="grid gap-1">
								<Label>{t('myaaw.settings.shell_mode')}</Label>
								<p class="text-xs text-muted-foreground">{t('myaaw.settings.shell_mode_desc')}</p>
							</div>
							<div class="grid grid-cols-1 items-start gap-3 sm:grid-cols-2">
								{#each shellModeOptions as option (option.value)}
									{@render shellTile(option)}
								{/each}
							</div>
						</div>
					</CardContent>
				</Card>
			{:else if activeGroup === 'chat'}
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

						<div class="grid gap-4 border-t pt-5">
							<div class="grid gap-1">
								<Label>{t('myaaw.settings.chat_while_streaming')}</Label>
								<p class="text-xs text-muted-foreground">
									{t('myaaw.settings.chat_while_streaming_desc')}
								</p>
							</div>
							<div class="flex flex-wrap gap-2">
								{@render toggle(
									t('myaaw.settings.queue_messages'),
									t('myaaw.settings.queue_messages_desc'),
									preferences.value.queueMessages,
									setQueueMessages
								)}
								{@render toggle(
									t('myaaw.settings.interrupt_on_send'),
									t('myaaw.settings.interrupt_on_send_desc'),
									preferences.value.interruptOnSend,
									setInterruptOnSend
								)}
							</div>
						</div>

						<div class="flex flex-wrap gap-2 border-t pt-5">
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
							{@render toggle(
								t('myaaw.settings.run_mascot'),
								t('myaaw.settings.run_mascot_desc'),
								preferences.value.showRunMascot,
								(next) => preferences.update({ showRunMascot: next })
							)}
						</div>
					</CardContent>
				</Card>
			{:else if activeGroup === 'voice'}
				<Card>
					<CardHeader>
						<CardTitle class="flex items-center gap-2">
							<Volume2 class="h-4 w-4" />
							{t('myaaw.settings.voice')}
						</CardTitle>
						<CardDescription>{t('myaaw.settings.voice_desc')}</CardDescription>
					</CardHeader>
					<CardContent class="space-y-5">
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
									onchange={(event) =>
										preferences.update({ speechVoice: event.currentTarget.value })}
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

						<div class="grid gap-2 border-t pt-5 sm:max-w-xs">
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
					</CardContent>
				</Card>
			{:else if activeGroup === 'notifications'}
				<Card>
					<CardHeader>
						<CardTitle class="flex items-center gap-2">
							<Bell class="h-4 w-4" />
							{t('myaaw.settings.notifications')}
						</CardTitle>
						<CardDescription>{t('myaaw.settings.notifications_desc')}</CardDescription>
					</CardHeader>
					<CardContent class="space-y-5">
						<div class="grid gap-3">
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
									onchange={(event) =>
										preferences.update({ notifySound: event.currentTarget.value })}
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
											playNotification(
												preferences.value.notifySound,
												preferences.value.notifyVolume
											)}
									>
										{t('myaaw.settings.notify_preview')}
									</Button>
								</div>
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
									<Button
										variant="outline"
										size="sm"
										class="w-fit"
										onclick={testBrowserNotification}
									>
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
					</CardContent>
				</Card>
			{:else if activeGroup === 'accessibility'}
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
								oninput={(event) =>
									preferences.update({ textScale: Number(event.currentTarget.value) })}
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
								t('myaaw.settings.connection_badge'),
								t('myaaw.settings.connection_badge_desc'),
								preferences.value.showConnectionBadge,
								(next) => preferences.update({ showConnectionBadge: next })
							)}
						</div>
					</CardContent>
				</Card>
			{:else if activeGroup === 'connection'}
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
						<div class="grid gap-2 border-t pt-4">
							{@render toggle(
								t('myaaw.settings.host_badge'),
								t('myaaw.settings.host_badge_desc'),
								preferences.value.showHostBadge,
								(next) => preferences.update({ showHostBadge: next })
							)}
						</div>
					</CardContent>
				</Card>
			{/if}
		</div>
	</section>
</div>
