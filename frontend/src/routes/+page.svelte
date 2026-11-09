<script lang="ts">
	import {
		ArrowUpRight,
		Check,
		ChevronDown,
		HeartPulse,
		Monitor,
		Palette,
		PawPrint,
		RadioTower,
		Send,
		Terminal
	} from 'lucide-svelte';
	import { mode, theme, userPrefersMode } from 'mode-watcher';
	import { reveal } from '$lib/actions/reveal';
	import type { BloubColor, BloubMood } from '$lib/bloub';
	import Bloub from '$lib/components/common/Bloub.svelte';
	import CopyButton from '$lib/components/common/CopyButton.svelte';
	import DiscordIcon from '$lib/components/common/DiscordIcon.svelte';
	import SEO from '$lib/components/common/SEO.svelte';
	import AgentField from '$lib/components/landing/AgentField.svelte';
	import RunStream from '$lib/components/landing/RunStream.svelte';
	import RunPreview from '$lib/components/landing/RunPreview.svelte';
	import ScrollTrail from '$lib/components/landing/ScrollTrail.svelte';
	import { Button } from '$lib/components/ui/button';
	import {
		DropdownMenu,
		DropdownMenuContent,
		DropdownMenuGroup,
		DropdownMenuGroupHeading,
		DropdownMenuItem,
		DropdownMenuTrigger
	} from '$lib/components/ui/dropdown-menu';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { isAuthenticated } from '$lib/stores';
	import { preferences } from '$lib/stores/preferences.svelte';
	import { APPEARANCE, applyAppearance, isAppearanceActive, MODES, THEMES } from '$lib/theme';

	const { t, locale, availableLocales, changeLocale } = useTranslation();

	const PANEL = 'frost-card bg-card text-card-foreground ring-(--surface-ring) ring-1 rounded-xl';

	const INSTALL =
		'curl -fsSL https://raw.githubusercontent.com/Shiyinq/myaaw/main/install.sh | bash';
	const REPO = 'https://github.com/Shiyinq/myaaw';

	const MARQUEE = [
		'bash',
		'filesystem',
		'python',
		'skills',
		'heartbeat',
		'cron',
		'approvals',
		'memory',
		'sqlite',
		'sse',
		'telegram',
		'discord',
		'mcp',
		'a2a',
		'rest api'
	];

	const CAPABILITIES = ['provider', 'tools', 'extensions', 'delegation', 'schedule', 'backup'];
	const SETUP_STEPS = ['install', 'agent', 'api', 'dashboard'];

	interface FleetAgent {
		name: string;
		role: string;
		color: BloubColor;
		mood: BloubMood;
		hover: BloubMood;
		port: string;
		pid: string;
		runs: string;
		skills: string;
		uptime: string;
		channels: string[];
	}

	const FLEET: FleetAgent[] = [
		{
			name: 'Myaaw',
			role: 'default',
			color: 'encre',
			mood: 'attentif',
			hover: 'heureux',
			port: '8080',
			pid: '48120',
			runs: '128',
			skills: '6',
			uptime: '4d 12h',
			channels: ['telegram', 'web']
		},
		{
			name: 'Myfriend',
			role: 'myfriend',
			color: 'violet',
			mood: 'heureux',
			hover: 'excite',
			port: '8081',
			pid: '48222',
			runs: '42',
			skills: '2',
			uptime: '1d 03h',
			channels: ['discord', 'cli']
		},
		{
			name: 'Nightshift',
			role: 'nightshift',
			color: 'bleu',
			mood: 'somnolent',
			hover: 'surpris',
			port: '8082',
			pid: '48310',
			runs: '17',
			skills: '4',
			uptime: '21d 08h',
			channels: ['heartbeat', 'cron']
		}
	];

	const RULES = ['item_1', 'item_2', 'item_3', 'item_4', 'item_5', 'item_6'];
	const NOTES = ['note_1', 'note_2', 'note_3'];

	let hovered = $state('');

	const signedIn = $derived(isAuthenticated.value);
	const dark = $derived(mode.current === 'dark');
	const signInHref = $derived(signedIn ? '/app' : '/login');
	const signInLabel = $derived(signedIn ? t('landing.hero.primary') : t('landing.nav.signin'));
	const activeAppearance = $derived(
		APPEARANCE.find((option) =>
			isAppearanceActive(option, userPrefersMode.current ?? 'system', theme.current)
		)
	);
	function handleNav(event: MouseEvent, id: string) {
		event.preventDefault();
		const el = document.getElementById(id);
		if (!el) return;
		el.scrollIntoView({
			behavior: preferences.value.reduceMotion ? 'auto' : 'smooth',
			block: 'start'
		});
		history.replaceState(null, '', `#${id}`);
	}

	function handleHome(event: MouseEvent) {
		// The logo already points at `/`; on the landing page it should glide home
		// instead of reloading the route.
		if (window.location.pathname !== '/') return;
		event.preventDefault();
		window.scrollTo({
			top: 0,
			behavior: preferences.value.reduceMotion ? 'auto' : 'smooth'
		});
		history.replaceState(null, '', '/');
	}
</script>

<SEO title={t('landing.meta.title')} description={t('landing.meta.description')} />

<header class="fixed inset-x-0 top-0 z-40 border-b border-border/70">
	<div class="frost-chrome bg-background/80 backdrop-blur-md">
		<div class="mx-auto flex h-14 max-w-6xl items-center gap-3 px-4 sm:px-6">
			<a
				href="/"
				onclick={handleHome}
				class="flex items-center gap-2 outline-none focus-visible:ring-2 focus-visible:ring-ring/60"
			>
				<img src="/logo.png" alt="" width="24" height="24" class="size-6 rounded-md object-cover" />
				<span class="text-sm font-bold tracking-tight">Myaaw</span>
			</a>
			<span
				class="mono hidden rounded border border-border/70 px-1.5 py-0.5 text-[10px] text-muted-foreground lg:inline"
				>{t('landing.footer.tagline')}</span
			>

			<nav class="ml-auto hidden items-center gap-5 md:flex">
				<a
					class="label text-muted-foreground transition-colors hover:text-foreground"
					href="#loop"
					onclick={(event) => handleNav(event, 'loop')}
					><span class="text-foreground/50">01</span> {t('landing.nav.loop')}</a
				>
				<a
					class="label text-muted-foreground transition-colors hover:text-foreground"
					href="#fleet"
					onclick={(event) => handleNav(event, 'fleet')}
					><span class="text-foreground/50">02</span> {t('landing.nav.fleet')}</a
				>
				<a
					class="label text-muted-foreground transition-colors hover:text-foreground"
					href="#doors"
					onclick={(event) => handleNav(event, 'doors')}
					><span class="text-foreground/50">03</span> {t('landing.nav.doors')}</a
				>
				<a
					class="label text-muted-foreground transition-colors hover:text-foreground"
					href="#rules"
					onclick={(event) => handleNav(event, 'rules')}
					><span class="text-foreground/50">04</span> {t('landing.nav.rules')}</a
				>
				<a
					class="label hidden text-muted-foreground transition-colors hover:text-foreground lg:inline"
					href="#closing"
					onclick={(event) => handleNav(event, 'closing')}
					><span class="text-foreground/50">05</span> {t('landing.nav.install')}</a
				>
			</nav>

			<div class="ml-auto flex items-center gap-2 md:ml-4">
				<div
					class="flex items-center rounded-md border border-border/70 p-0.5"
					role="group"
					aria-label={t('landing.nav.language')}
				>
					{#each availableLocales as option (option.code)}
						<button
							type="button"
							aria-pressed={locale.value === option.code}
							onclick={() => changeLocale(option.code)}
							class="mono rounded px-1.5 py-0.5 text-[10px] tracking-widest uppercase transition-colors {locale.value ===
							option.code
								? 'bg-accent text-accent-foreground'
								: 'text-muted-foreground hover:text-foreground'}"
						>
							{option.code}
						</button>
					{/each}
				</div>

				<Button href={signInHref} size="sm" class="gap-1.5">
					<span class="hidden sm:inline">{signInLabel}</span>
					<span class="sm:hidden">
						{signedIn ? t('landing.nav.open_short') : t('landing.nav.signin')}
					</span>
					<ArrowUpRight class="size-3.5" />
				</Button>
			</div>
		</div>
	</div>
</header>

<main id="main" class="relative pt-14">
	<section class="relative overflow-hidden border-b border-border/60">
		<AgentField class="mask-[radial-gradient(120%_105%_at_62%_18%,black_32%,transparent_82%)]" />

		<div
			class="relative mx-auto grid max-w-6xl gap-12 px-4 pt-12 pb-16 sm:px-6 lg:grid-cols-[1.05fr_0.95fr] lg:items-center lg:gap-14 lg:pt-20 lg:pb-24"
		>
			<div class="hero-copy min-w-0">
				<p class="label flex flex-wrap items-center gap-2 text-muted-foreground" use:reveal>
					<span class="live-dot"></span>
					{t('landing.hero.eyebrow')}
					<span class="text-border">/</span>
					<span class="tracking-normal normal-case">{t('landing.hero.backend_badge')}</span>
				</p>

				<h1 class="mt-6">
					<span
						class="block text-xl font-medium tracking-tight text-muted-foreground sm:text-2xl lg:text-3xl"
						use:reveal={{ delay: 60 }}>{t('landing.hero.lead')}</span
					>
					<span
						class="hero-heading mt-2 block text-[2.3rem] leading-[1.06] font-bold tracking-[-0.045em] sm:text-6xl xl:text-7xl"
					>
						<span class="hero-line" use:reveal={{ delay: 120 }}>
							<span class="glyph" aria-hidden="true"><span class="glyph-node"></span></span>
							<span class="hero-text">{t('landing.hero.line_process')}</span>
						</span>
						<span class="hero-line" use:reveal={{ delay: 190 }}>
							<span class="glyph" aria-hidden="true">
								<span class="glyph-bars"><i></i><i></i><i></i></span>
							</span>
							<span class="hero-text">{t('landing.hero.line_memory')}</span>
						</span>
						<span class="hero-line" use:reveal={{ delay: 260 }}>
							<span class="glyph glyph-arrow" aria-hidden="true">
								<ArrowUpRight class="size-[0.32em]" />
							</span>
							<span class="hero-text">{t('landing.hero.line_door')}</span>
						</span>
					</span>
				</h1>

				<p
					class="mt-6 max-w-xl text-[15px] leading-relaxed text-muted-foreground sm:text-base"
					use:reveal={{ delay: 300 }}
				>
					{t('landing.hero.body')}
				</p>

				<div class="mt-8 flex flex-wrap items-center gap-3" use:reveal={{ delay: 340 }}>
					<Button href={signInHref} size="lg" class="gap-2">
						{signInLabel}
						<ArrowUpRight class="size-4" />
					</Button>
					<Button
						href={REPO}
						target="_blank"
						rel="noreferrer"
						variant="outline"
						size="lg"
						class="gap-2"
					>
						<svg viewBox="0 0 24 24" class="size-4 fill-current" aria-hidden="true">
							<path
								d="M12 .5a11.5 11.5 0 0 0-3.64 22.41c.58.1.79-.25.79-.55v-2.17c-3.2.7-3.88-1.36-3.88-1.36-.52-1.34-1.28-1.7-1.28-1.7-1.05-.72.08-.7.08-.7 1.16.08 1.77 1.19 1.77 1.19 1.03 1.77 2.7 1.26 3.36.96.1-.75.4-1.26.73-1.55-2.55-.29-5.24-1.28-5.24-5.69 0-1.26.45-2.29 1.19-3.09-.12-.29-.52-1.46.11-3.05 0 0 .97-.31 3.18 1.18a11 11 0 0 1 5.79 0c2.2-1.49 3.17-1.18 3.17-1.18.63 1.59.23 2.76.12 3.05.74.8 1.18 1.83 1.18 3.09 0 4.42-2.69 5.39-5.26 5.68.41.36.78 1.06.78 2.14v3.17c0 .31.21.66.8.55A11.5 11.5 0 0 0 12 .5Z"
							/>
						</svg>
						{t('landing.hero.secondary')}
					</Button>
				</div>

				<p class="label mt-6 text-muted-foreground" use:reveal={{ delay: 380 }}>
					{t('landing.hero.meta')}
				</p>
			</div>

			<div class="min-w-0" use:reveal={{ delay: 200, distance: 20 }}>
				<RunStream />
				<p class="label mt-3 flex items-center gap-2 text-muted-foreground">
					<span class="scroll-line" aria-hidden="true"></span>
					{t('landing.hero.scroll')}
				</p>
			</div>
		</div>
	</section>

	<div class="marquee border-b border-border/60 py-3" aria-hidden="true">
		<div class="marquee-track">
			{#each [0, 1] as copy (copy)}
				<div class="flex shrink-0 items-center gap-6 pr-6">
					{#each MARQUEE as item (item)}
						<span class="label flex items-center gap-2 text-muted-foreground">
							<PawPrint class="size-3 opacity-40" />
							{item}
						</span>
					{/each}
				</div>
			{/each}
		</div>
	</div>

	<section id="loop" class="scroll-mt-20 border-b border-border/60">
		<div class="mx-auto max-w-6xl px-4 py-16 sm:px-6 lg:py-24">
			<div class="grid gap-4 lg:grid-cols-[0.34fr_1fr] lg:gap-12" use:reveal>
				<p class="label text-muted-foreground">
					<span class="text-foreground">01</span> / {t('landing.loop.index')}
				</p>
				<div>
					<h2 class="max-w-2xl text-3xl font-bold tracking-[-0.035em] sm:text-4xl">
						{t('landing.loop.title')}
					</h2>
					<p class="mt-4 max-w-xl text-muted-foreground">{t('landing.loop.body')}</p>
				</div>
			</div>

			<div class="mt-10 grid gap-8 lg:mt-14 lg:grid-cols-[0.34fr_1fr] lg:gap-12">
				<div class="space-y-6" use:reveal>
					{#each NOTES as note, index (note)}
						<div class="border-t border-border/70 pt-4">
							<p class="label text-muted-foreground">
								{String(index + 1).padStart(2, '0')}
							</p>
							<h3 class="mt-2 text-base font-semibold">{t(`landing.loop.${note}_title`)}</h3>
							<p class="mt-1.5 text-sm leading-relaxed text-muted-foreground">
								{t(`landing.loop.${note}_body`)}
							</p>
						</div>
					{/each}
				</div>

				<div use:reveal={{ delay: 100 }}>
					<RunPreview />
					<p class="mt-3 text-xs leading-relaxed text-muted-foreground">
						{t('landing.loop.example_note')}
					</p>
				</div>
			</div>
		</div>
	</section>

	<section id="fleet" class="scroll-mt-20 border-b border-border/60">
		<div class="mx-auto max-w-6xl px-4 py-16 sm:px-6 lg:py-24">
			<div class="grid gap-4 lg:grid-cols-[0.34fr_1fr] lg:gap-12" use:reveal>
				<p class="label text-muted-foreground">
					<span class="text-foreground">02</span> / {t('landing.fleet.index')}
				</p>
				<div>
					<h2 class="max-w-2xl text-3xl font-bold tracking-[-0.035em] sm:text-4xl">
						{t('landing.fleet.title')}
					</h2>
					<p class="mt-4 max-w-xl text-muted-foreground">{t('landing.fleet.body')}</p>
				</div>
			</div>

			<p class="mt-8 text-xs leading-relaxed text-muted-foreground" use:reveal>
				{t('landing.fleet.example_note')}
			</p>
			<div class="mt-4 grid gap-4 md:grid-cols-3">
				{#each FLEET as agent, index (agent.name)}
					<div use:reveal={{ delay: index * 90 }}>
						<article
							class="{PANEL} group flex h-full flex-col p-5 transition-transform duration-300 hover:-translate-y-1"
							data-slot="card"
							onmouseenter={() => (hovered = agent.name)}
							onmouseleave={() => (hovered = '')}
						>
							<div class="flex items-start justify-between gap-3">
								<div class="flex min-w-0 items-center gap-3">
									<div
										class="grid size-14 shrink-0 place-items-center rounded-lg border border-border/70 bg-background/40"
									>
										<Bloub
											mood={hovered === agent.name ? agent.hover : agent.mood}
											color={dark && agent.color === 'encre' ? 'orange' : agent.color}
											size={50}
											fade
										/>
									</div>
									<div class="min-w-0">
										<h3 class="truncate text-base font-semibold">{agent.name}</h3>
										<p class="label mt-1 truncate text-muted-foreground">
											{t(`landing.fleet.role_${agent.role}`)}
										</p>
									</div>
								</div>
								<span class="status-led" aria-hidden="true"></span>
							</div>

							<dl class="mt-5 grid grid-cols-3 gap-2 border-t border-border/70 pt-4">
								<div>
									<dt class="label text-muted-foreground">{t('landing.fleet.runs')}</dt>
									<dd class="mono mt-1 text-lg font-semibold">{agent.runs}</dd>
								</div>
								<div>
									<dt class="label text-muted-foreground">{t('landing.fleet.skills')}</dt>
									<dd class="mono mt-1 text-lg font-semibold">{agent.skills}</dd>
								</div>
								<div>
									<dt class="label text-muted-foreground">{t('landing.fleet.uptime')}</dt>
									<dd class="mono mt-1 text-lg font-semibold">{agent.uptime}</dd>
								</div>
							</dl>

							<div class="mt-4 flex flex-wrap gap-1.5">
								{#each agent.channels as channel (channel)}
									<span
										class="mono rounded border border-border/70 px-1.5 py-0.5 text-[10px] text-muted-foreground"
										>{channel}</span
									>
								{/each}
							</div>

							<p class="mono mt-auto pt-4 text-[10px] text-muted-foreground">
								pid {agent.pid} · :{agent.port}
							</p>
						</article>
					</div>
				{/each}
			</div>

			<div class="mt-6 flex flex-wrap items-center gap-x-4 gap-y-2" use:reveal>
				<p
					class="mono flex items-center gap-2 rounded-md border border-border/70 px-2.5 py-1.5 text-[11px]"
				>
					{t('landing.fleet.process_caption')}
					<span class="shell-caret" aria-hidden="true"></span>
				</p>
				<p class="label text-muted-foreground">{t('landing.fleet.process_note')}</p>
			</div>

			<div class="mt-14 border-t border-border/70 pt-8" use:reveal>
				<h3 class="max-w-2xl text-2xl font-semibold tracking-tight">
					{t('landing.capabilities.title')}
				</h3>
				<p class="mt-3 max-w-2xl text-sm leading-relaxed text-muted-foreground">
					{t('landing.capabilities.body')}
				</p>
			</div>
			<div class="mt-6 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
				{#each CAPABILITIES as capability, index (capability)}
					<article class="{PANEL} min-w-0 p-5" data-slot="card" use:reveal={{ delay: index * 40 }}>
						<p class="label text-muted-foreground">
							{t(`landing.capabilities.${capability === 'backup' ? 'available' : 'configured'}`)}
						</p>
						<h4 class="mt-3 text-base font-semibold">
							{t(`landing.capabilities.${capability}_title`)}
						</h4>
						<p class="mt-2 text-sm leading-relaxed text-muted-foreground wrap-anywhere">
							{t(`landing.capabilities.${capability}_body`)}
						</p>
					</article>
				{/each}
			</div>
			<p class="mt-4 text-xs leading-relaxed text-muted-foreground" use:reveal>
				{t('landing.capabilities.computer_note')}
			</p>
		</div>
	</section>

	<section id="doors" class="scroll-mt-20 border-b border-border/60">
		<div class="mx-auto max-w-6xl px-4 py-16 sm:px-6 lg:py-24">
			<div class="grid gap-4 lg:grid-cols-[0.34fr_1fr] lg:gap-12" use:reveal>
				<p class="label text-muted-foreground">
					<span class="text-foreground">03</span> / {t('landing.doors.index')}
				</p>
				<div>
					<h2 class="max-w-2xl text-3xl font-bold tracking-[-0.035em] sm:text-4xl">
						{t('landing.doors.title')}
					</h2>
					<p class="mt-4 max-w-xl text-muted-foreground">{t('landing.doors.body')}</p>
				</div>
			</div>

			<div class="mt-10 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
				<div class="{PANEL} group flex gap-3 p-4" data-slot="card" use:reveal>
					<span class="door-icon"><Send class="size-4" /></span>
					<div class="min-w-0">
						<div class="flex items-baseline justify-between gap-2">
							<h3 class="text-sm font-semibold">Telegram</h3>
							<span class="label text-muted-foreground">bot</span>
						</div>
						<p class="mt-1 text-xs leading-relaxed text-muted-foreground">
							{t('landing.doors.telegram')}
						</p>
					</div>
				</div>

				<div class="{PANEL} group flex gap-3 p-4" data-slot="card" use:reveal={{ delay: 60 }}>
					<span class="door-icon"><DiscordIcon class="size-4" /></span>
					<div class="min-w-0">
						<div class="flex items-baseline justify-between gap-2">
							<h3 class="text-sm font-semibold">Discord</h3>
							<span class="label text-muted-foreground">bot</span>
						</div>
						<p class="mt-1 text-xs leading-relaxed text-muted-foreground">
							{t('landing.doors.discord')}
						</p>
					</div>
				</div>

				<div class="{PANEL} group flex gap-3 p-4" data-slot="card" use:reveal={{ delay: 120 }}>
					<span class="door-icon"><Terminal class="size-4" /></span>
					<div class="min-w-0">
						<div class="flex items-baseline justify-between gap-2">
							<h3 class="text-sm font-semibold">Terminal</h3>
							<span class="label text-muted-foreground">tui</span>
						</div>
						<p class="mt-1 text-xs leading-relaxed text-muted-foreground">
							{t('landing.doors.terminal')}
						</p>
					</div>
				</div>

				<div class="{PANEL} group flex gap-3 p-4" data-slot="card" use:reveal={{ delay: 180 }}>
					<span class="door-icon"><Monitor class="size-4" /></span>
					<div class="min-w-0">
						<div class="flex items-baseline justify-between gap-2">
							<h3 class="text-sm font-semibold">{t('landing.doors.web_title')}</h3>
							<span class="label text-muted-foreground">web</span>
						</div>
						<p class="mt-1 text-xs leading-relaxed text-muted-foreground">
							{t('landing.doors.web')}
						</p>
					</div>
				</div>

				<div class="{PANEL} group flex gap-3 p-4" data-slot="card" use:reveal={{ delay: 240 }}>
					<span class="door-icon"><RadioTower class="size-4" /></span>
					<div class="min-w-0">
						<div class="flex items-baseline justify-between gap-2">
							<h3 class="text-sm font-semibold">API</h3>
							<span class="label text-muted-foreground">http</span>
						</div>
						<p class="mt-1 text-xs leading-relaxed text-muted-foreground">
							{t('landing.doors.api')}
						</p>
					</div>
				</div>

				<div class="{PANEL} group flex gap-3 p-4" data-slot="card" use:reveal={{ delay: 300 }}>
					<span class="door-icon"><HeartPulse class="size-4" /></span>
					<div class="min-w-0">
						<div class="flex items-baseline justify-between gap-2">
							<h3 class="text-sm font-semibold">Heartbeat</h3>
							<span class="label text-muted-foreground">auto</span>
						</div>
						<p class="mt-1 text-xs leading-relaxed text-muted-foreground">
							{t('landing.doors.heartbeat')}
						</p>
					</div>
				</div>
			</div>
		</div>
	</section>

	<section id="rules" class="scroll-mt-20 border-b border-border/60">
		<div class="mx-auto max-w-6xl px-4 py-16 sm:px-6 lg:py-24">
			<div class="grid gap-4 lg:grid-cols-[0.34fr_1fr] lg:gap-12" use:reveal>
				<p class="label text-muted-foreground">
					<span class="text-foreground">04</span> / {t('landing.rules.index')}
				</p>
				<div>
					<h2 class="max-w-2xl text-3xl font-bold tracking-[-0.035em] sm:text-4xl">
						{t('landing.rules.title')}
					</h2>
					<p class="mt-4 max-w-xl text-muted-foreground">{t('landing.rules.body')}</p>
				</div>
			</div>

			<ol class="mt-10 border-t border-border/70">
				{#each RULES as rule, index (rule)}
					<li
						class="grid gap-2 border-b border-border/70 py-5 transition-colors hover:bg-muted/30 sm:grid-cols-[3rem_1fr_1.35fr] sm:gap-6"
						use:reveal={{ delay: index * 60 }}
					>
						<span class="label text-muted-foreground">{String(index + 1).padStart(2, '0')}</span>
						<h3 class="text-base font-semibold sm:text-lg">
							{t(`landing.rules.${rule}_title`)}
						</h3>
						<p class="text-sm leading-relaxed text-muted-foreground">
							{t(`landing.rules.${rule}_body`)}
						</p>
					</li>
				{/each}
			</ol>
		</div>
	</section>

	<section id="closing" class="relative scroll-mt-20 overflow-hidden">
		<div class="mx-auto max-w-6xl px-4 py-16 sm:px-6 lg:py-24">
			<div class="grid items-center gap-10 lg:grid-cols-[1fr_auto]">
				<div use:reveal>
					<p class="label text-muted-foreground">
						<span class="text-foreground">05</span> / {t('landing.closing.index')}
					</p>
					<h2 class="mt-4 text-3xl font-bold tracking-[-0.035em] sm:text-5xl">
						<span class="block">{t('landing.closing.title_1')}</span>
						<span class="block text-muted-foreground">{t('landing.closing.title_2')}</span>
					</h2>
					<p class="mt-4 max-w-lg text-muted-foreground">{t('landing.closing.body')}</p>

					<div class="mt-8 flex flex-wrap items-center gap-3">
						<Button href={signInHref} size="lg" class="gap-2">
							{signInLabel}
							<ArrowUpRight class="size-4" />
						</Button>
						<Button href={REPO} target="_blank" rel="noreferrer" variant="outline" size="lg">
							{t('landing.closing.secondary')}
						</Button>
					</div>
				</div>

				<div class="flex flex-col items-center gap-4" use:reveal={{ delay: 120 }}>
					<Bloub cycle="wink" size={144} color={dark ? 'orange' : 'encre'} class="max-sm:hidden" />
					<p class="label text-center text-muted-foreground">{t('landing.closing.signature')}</p>
				</div>
			</div>

			<div class="{PANEL} mt-10 overflow-hidden" data-slot="card" use:reveal={{ delay: 80 }}>
				<div class="flex items-center justify-between border-b border-border/70 px-4 py-2">
					<span class="label text-muted-foreground">{t('landing.closing.install_label')}</span>
					<CopyButton text={INSTALL} />
				</div>
				<pre class="mono overflow-x-auto px-4 py-4 text-xs leading-relaxed"><code
						><span class="text-muted-foreground">$</span> {INSTALL}</code
					></pre>
				<div class="space-y-2 border-t border-border/70 px-4 py-3">
					<p class="text-xs leading-relaxed text-muted-foreground">
						{t('landing.closing.install_hint')}{' '}
						<a
							href={`${REPO}/releases/latest`}
							target="_blank"
							rel="noreferrer"
							class="font-medium underline underline-offset-4 hover:text-foreground"
							>{t('landing.closing.releases')}</a
						>
					</p>
				</div>
			</div>

			<ol class="mt-8 grid gap-5 sm:grid-cols-2 lg:grid-cols-4">
				{#each SETUP_STEPS as step, index (step)}
					<li class="min-w-0 border-t border-border/70 pt-4" use:reveal={{ delay: index * 50 }}>
						<p class="label text-muted-foreground">{String(index + 1).padStart(2, '0')}</p>
						<h3 class="mt-2 text-sm font-semibold">{t(`landing.closing.steps.${step}_title`)}</h3>
						<p class="mt-2 text-xs leading-relaxed text-muted-foreground">
							{t(`landing.closing.steps.${step}_body`)}
						</p>
					</li>
				{/each}
			</ol>
			<a
				class="mt-6 inline-flex items-center gap-1.5 text-sm font-medium underline underline-offset-4"
				href={`${REPO}/blob/main/frontend/README.md`}
				target="_blank"
				rel="noreferrer">{t('landing.closing.guide')} <ArrowUpRight class="size-3.5" /></a
			>
		</div>
	</section>
</main>

<footer class="border-t border-border/60">
	<div
		class="mx-auto flex max-w-6xl flex-col gap-4 px-4 py-8 sm:px-6 md:flex-row md:items-center md:justify-between"
	>
		<div class="flex items-center gap-2">
			<img src="/logo.png" alt="" width="20" height="20" class="size-5 rounded object-cover" />
			<span class="text-sm font-semibold">Myaaw</span>
			<span class="label text-muted-foreground">{t('landing.footer.tagline')}</span>
		</div>

		<nav class="flex flex-wrap items-center gap-4 text-xs">
			<a
				class="text-muted-foreground transition-colors hover:text-foreground"
				href={REPO}
				target="_blank"
				rel="noreferrer">{t('landing.footer.source')}</a
			>
			<a
				class="text-muted-foreground transition-colors hover:text-foreground"
				href={`${REPO}/tree/main/docs`}
				target="_blank"
				rel="noreferrer">{t('landing.footer.docs')}</a
			>
			<a
				class="text-muted-foreground transition-colors hover:text-foreground"
				href={signedIn ? '/app/settings' : '/login'}>{t('landing.footer.settings')}</a
			>
		</nav>

		<div class="flex flex-wrap items-center gap-2">
			<DropdownMenu>
				<DropdownMenuTrigger
					class="group flex items-center gap-1.5 rounded-md border border-border/70 bg-background/40 py-1 pr-2 pl-2 text-muted-foreground transition-colors outline-none select-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/50 data-[state=open]:bg-accent data-[state=open]:text-accent-foreground"
				>
					<Palette class="size-3.5 shrink-0" aria-hidden="true" />
					<span class="label">{t('landing.footer.theme')}</span>
					<span class="mono max-w-40 truncate text-[11px] text-foreground">
						{activeAppearance ? t(activeAppearance.labelKey) : '—'}
					</span>
					<ChevronDown
						class="size-3 shrink-0 opacity-60 transition-transform group-data-[state=open]:rotate-180"
						aria-hidden="true"
					/>
				</DropdownMenuTrigger>

				<DropdownMenuContent side="top" align="start" sideOffset={8} class="max-h-88 w-76 p-1.5">
					<DropdownMenuGroup>
						<DropdownMenuGroupHeading
							class="mono px-2 pt-1 pb-0.5 text-[10px] font-normal tracking-[0.16em] text-muted-foreground uppercase"
						>
							{t('landing.footer.modes')}
						</DropdownMenuGroupHeading>
						<div class="grid grid-cols-2 gap-0.5">
							{#each MODES as option (option.id)}
								{@const on = activeAppearance?.id === option.id}
								<DropdownMenuItem
									class="gap-2 px-1.5 py-1.5 {on ? 'bg-accent text-accent-foreground' : ''}"
									onSelect={() => applyAppearance(option)}
								>
									<span class="swatch" style={`background: ${option.swatch.bg}`}>
										<span style={`background: ${option.swatch.card}`}></span>
										<i style={`background: ${option.swatch.accent}`}></i>
									</span>
									<span class="min-w-0 flex-1 truncate text-[11px]">
										{t(option.labelKey)}
									</span>
									{#if on}<Check class="size-3.5 shrink-0" />{/if}
								</DropdownMenuItem>
							{/each}
						</div>
					</DropdownMenuGroup>

					<div class="my-1.5 h-px bg-border/60"></div>

					<DropdownMenuGroup>
						<DropdownMenuGroupHeading
							class="mono px-2 pt-1 pb-0.5 text-[10px] font-normal tracking-[0.16em] text-muted-foreground uppercase"
						>
							{t('landing.footer.themes')}
						</DropdownMenuGroupHeading>
						<div class="grid grid-cols-2 gap-0.5">
							{#each THEMES as option (option.id)}
								{@const on = activeAppearance?.id === option.id}
								<DropdownMenuItem
									class="gap-2 px-1.5 py-1.5 {on ? 'bg-accent text-accent-foreground' : ''}"
									onSelect={() => applyAppearance(option)}
								>
									<span class="swatch" style={`background: ${option.swatch.bg}`}>
										<span style={`background: ${option.swatch.card}`}></span>
										<i style={`background: ${option.swatch.accent}`}></i>
									</span>
									<span class="min-w-0 flex-1 truncate text-[11px]">
										{t(option.labelKey)}
									</span>
									{#if on}<Check class="size-3.5 shrink-0" />{/if}
								</DropdownMenuItem>
							{/each}
						</div>
					</DropdownMenuGroup>
				</DropdownMenuContent>
			</DropdownMenu>

			<span class="label text-muted-foreground">{t('landing.footer.license')}</span>
		</div>
	</div>
</footer>

<ScrollTrail />

<style>
	.mono {
		font-family: var(--font-ui-mono);
		font-variant-ligatures: none;
	}

	.label {
		font-family: var(--font-ui-mono);
		font-size: 0.6875rem;
		line-height: 1.45;
		letter-spacing: 0.16em;
		text-transform: uppercase;
	}

	/* The reveal action toggles these; the page owns the distance and easing. */
	.reveal {
		opacity: 0;
		transform: translateY(var(--reveal-distance, 14px));
		transition:
			opacity 0.7s cubic-bezier(0.22, 1, 0.36, 1) var(--reveal-delay, 0ms),
			transform 0.7s cubic-bezier(0.22, 1, 0.36, 1) var(--reveal-delay, 0ms);
	}

	.reveal.is-revealed {
		opacity: 1;
		transform: none;
	}

	.hero-copy {
		container-type: inline-size;
	}

	/* Monospace themes need a smaller heading to keep each phrase on one line. */
	:global(html[data-theme='terminal']) .hero-heading,
	:global(html[data-theme='typewriter']) .hero-heading {
		font-size: min(4.5rem, 10cqi);
	}

	.hero-line {
		display: flex;
		align-items: center;
		gap: 0.14em;
	}

	.hero-text {
		min-width: 0;
	}

	.hero-line + .hero-line {
		margin-top: 0.04em;
	}

	.glyph {
		display: inline-flex;
		flex: none;
		align-items: center;
		justify-content: center;
		width: 0.46em;
		height: 0.46em;
		color: color-mix(in oklab, var(--foreground) 50%, transparent);
	}

	.glyph-node {
		position: relative;
		display: block;
		width: 0.11em;
		height: 0.11em;
		border-radius: 9999px;
		background: currentColor;
	}

	.glyph-node::after {
		content: '';
		position: absolute;
		inset: -0.08em;
		border-radius: 9999px;
		border: 1px solid currentColor;
		animation: glyph-ring 2.8s ease-out infinite;
	}

	@keyframes glyph-ring {
		0% {
			transform: scale(0.65);
			opacity: 0.65;
		}
		75%,
		100% {
			transform: scale(1.8);
			opacity: 0;
		}
	}

	.glyph-bars {
		display: flex;
		flex-direction: column;
		gap: 0.045em;
	}

	.glyph-bars i {
		display: block;
		height: 0.035em;
		border-radius: 9999px;
		background: currentColor;
		transform-origin: left center;
		animation: glyph-bars 2.6s ease-in-out infinite;
	}

	.glyph-bars i:nth-child(1) {
		width: 0.2em;
	}

	.glyph-bars i:nth-child(2) {
		width: 0.3em;
		animation-delay: 0.2s;
	}

	.glyph-bars i:nth-child(3) {
		width: 0.25em;
		animation-delay: 0.4s;
	}

	@keyframes glyph-bars {
		0%,
		100% {
			opacity: 0.35;
			transform: scaleX(0.8);
		}
		50% {
			opacity: 1;
			transform: scaleX(1);
		}
	}

	.glyph-arrow {
		animation: glyph-nudge 2.6s ease-in-out infinite;
	}

	@keyframes glyph-nudge {
		0%,
		100% {
			transform: translate(0, 0);
		}
		50% {
			transform: translate(0.03em, -0.03em);
		}
	}

	.live-dot {
		position: relative;
		width: 0.4rem;
		height: 0.4rem;
		border-radius: 9999px;
		background: var(--primary);
	}

	.live-dot::after {
		content: '';
		position: absolute;
		inset: -0.18rem;
		border-radius: 9999px;
		border: 1px solid var(--primary);
		opacity: 0.4;
		animation: live 2.4s ease-out infinite;
	}

	@keyframes live {
		0% {
			transform: scale(0.6);
			opacity: 0.5;
		}
		70%,
		100% {
			transform: scale(1.7);
			opacity: 0;
		}
	}

	.scroll-line {
		position: relative;
		display: inline-block;
		width: 1.5rem;
		height: 1px;
		overflow: hidden;
		background: var(--border);
	}

	.scroll-line::after {
		content: '';
		position: absolute;
		inset: 0;
		width: 0.6rem;
		background: var(--foreground);
		animation: scroll-line 2.4s ease-in-out infinite;
	}

	@keyframes scroll-line {
		0% {
			transform: translateX(-100%);
		}
		60%,
		100% {
			transform: translateX(280%);
		}
	}

	.marquee {
		overflow: hidden;
		mask-image: linear-gradient(to right, transparent, black 6%, black 94%, transparent);
		-webkit-mask-image: linear-gradient(to right, transparent, black 6%, black 94%, transparent);
	}

	.marquee-track {
		display: flex;
		width: max-content;
		animation: marquee 46s linear infinite;
	}

	.marquee:hover .marquee-track {
		animation-play-state: paused;
	}

	@keyframes marquee {
		from {
			transform: translateX(0);
		}
		to {
			transform: translateX(-50%);
		}
	}

	.status-led {
		width: 0.4rem;
		height: 0.4rem;
		margin-top: 0.4rem;
		border-radius: 9999px;
		background: var(--primary);
		box-shadow: 0 0 0 4px color-mix(in oklab, var(--primary) 14%, transparent);
	}

	.shell-caret {
		display: inline-block;
		width: 0.42rem;
		height: 0.9em;
		vertical-align: -0.1em;
		background: currentColor;
		animation: shell-caret 1.1s steps(1) infinite;
	}

	@keyframes shell-caret {
		0%,
		49% {
			opacity: 1;
		}
		50%,
		100% {
			opacity: 0;
		}
	}

	.door-icon {
		display: grid;
		flex: none;
		place-items: center;
		width: 2.25rem;
		height: 2.25rem;
		border: 1px solid color-mix(in oklab, var(--border) 75%, transparent);
		border-radius: calc(var(--radius) - 1px);
		background: color-mix(in oklab, var(--background) 45%, transparent);
		transition: transform 0.3s cubic-bezier(0.22, 1, 0.36, 1);
	}

	.swatch {
		position: relative;
		display: inline-block;
		flex: none;
		width: 1rem;
		height: 1rem;
		overflow: hidden;
		border-radius: 4px;
		border: 1px solid color-mix(in oklab, var(--foreground) 14%, transparent);
	}

	.swatch > span {
		position: absolute;
		inset: 2px;
		border-radius: 2px;
	}

	.swatch > i {
		position: absolute;
		bottom: 2px;
		left: 2px;
		width: 3px;
		height: 3px;
		border-radius: 9999px;
	}

	:global(.group:hover) .door-icon {
		transform: translateY(-1px) rotate(-4deg);
	}

	@media (prefers-reduced-motion: reduce) {
		.marquee-track,
		.shell-caret,
		.glyph-arrow,
		.glyph-bars i,
		.glyph-node::after,
		.live-dot::after,
		.scroll-line::after {
			animation: none;
		}

		.shell-caret {
			opacity: 1;
		}
	}

	:global(html[data-reduce-motion='true']) .marquee-track,
	:global(html[data-reduce-motion='true']) .shell-caret,
	:global(html[data-reduce-motion='true']) .glyph-arrow,
	:global(html[data-reduce-motion='true']) .glyph-bars i,
	:global(html[data-reduce-motion='true']) .glyph-node::after,
	:global(html[data-reduce-motion='true']) .live-dot::after,
	:global(html[data-reduce-motion='true']) .scroll-line::after {
		animation: none;
	}

	:global(html[data-reduce-motion='true']) .shell-caret {
		opacity: 1;
	}
</style>
