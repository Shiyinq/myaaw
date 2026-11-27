<script lang="ts">
	import { Check, ChevronDown, Palette } from 'lucide-svelte';
	import { theme, userPrefersMode } from 'mode-watcher';
	import {
		DropdownMenu,
		DropdownMenuContent,
		DropdownMenuGroup,
		DropdownMenuGroupHeading,
		DropdownMenuItem,
		DropdownMenuTrigger
	} from '$lib/components/ui/dropdown-menu';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { DOCS_URL, RELEASES_URL, REPO_URL } from '$lib/landing/sections';
	import { isAuthenticated } from '$lib/stores';
	import { APPEARANCE, applyAppearance, isAppearanceActive, MODES, THEMES } from '$lib/theme';

	const { t } = useTranslation();

	const signedIn = $derived(isAuthenticated.value);
	const activeAppearance = $derived(
		APPEARANCE.find((option) =>
			isAppearanceActive(option, userPrefersMode.current ?? 'system', theme.current)
		)
	);
</script>

<footer class="border-t border-border/60">
	<div
		class="mx-auto flex max-w-6xl flex-col gap-4 px-4 py-8 sm:px-6 md:flex-row md:items-center md:justify-between"
	>
		<div class="flex min-w-0 items-center gap-2">
			<img
				src="/logo.png"
				alt=""
				width="20"
				height="20"
				class="size-5 shrink-0 rounded object-cover"
			/>
			<span class="text-sm font-semibold text-foreground">Myaaw</span>
			<span class="landing-label truncate text-muted-foreground">{t('landing.footer.tagline')}</span
			>
		</div>

		<nav class="flex flex-wrap items-center gap-4 text-xs" aria-label="Footer">
			<a
				class="text-muted-foreground transition-colors hover:text-foreground"
				href={REPO_URL}
				target="_blank"
				rel="noreferrer"
			>
				{t('landing.footer.source')}
			</a>
			<a
				class="text-muted-foreground transition-colors hover:text-foreground"
				href={RELEASES_URL}
				target="_blank"
				rel="noreferrer"
			>
				Releases
			</a>
			<a
				class="text-muted-foreground transition-colors hover:text-foreground"
				href={DOCS_URL}
				target="_blank"
				rel="noreferrer"
			>
				{t('landing.footer.docs')}
			</a>
			<a
				class="text-muted-foreground transition-colors hover:text-foreground"
				href={signedIn ? '/app/settings' : '/login'}
			>
				{t('landing.footer.settings')}
			</a>
		</nav>

		<div class="flex flex-wrap items-center gap-2">
			<DropdownMenu>
				<DropdownMenuTrigger
					class="group flex items-center gap-1.5 rounded-md border border-border/70 bg-background/40 py-1 pr-2 pl-2 text-muted-foreground transition-colors outline-none select-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/50 data-[state=open]:bg-accent data-[state=open]:text-accent-foreground"
				>
					<Palette class="size-3.5 shrink-0" aria-hidden="true" />
					<span class="landing-label">{t('landing.footer.theme')}</span>
					<span class="landing-mono max-w-40 truncate text-[11px] text-foreground">
						{activeAppearance ? t(activeAppearance.labelKey) : '—'}
					</span>
					<ChevronDown
						class="size-3 shrink-0 opacity-60 transition-transform group-data-[state=open]:rotate-180"
						aria-hidden="true"
					/>
				</DropdownMenuTrigger>

				<DropdownMenuContent
					side="top"
					align="start"
					sideOffset={8}
					data-lenis-prevent
					class="max-h-88 w-76 p-1.5"
				>
					<DropdownMenuGroup>
						<DropdownMenuGroupHeading
							class="landing-mono px-2 pt-1 pb-0.5 text-[10px] font-normal tracking-[0.16em] text-muted-foreground uppercase"
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
							class="landing-mono px-2 pt-1 pb-0.5 text-[10px] font-normal tracking-[0.16em] text-muted-foreground uppercase"
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

			<span class="landing-label text-muted-foreground">{t('landing.footer.license')}</span>
		</div>
	</div>
</footer>

<style>
	.landing-mono {
		font-family: var(--font-ui-mono);
		font-variant-ligatures: none;
	}

	.landing-label {
		font-family: var(--font-ui-mono);
		font-size: 0.6875rem;
		line-height: 1.45;
		letter-spacing: 0.16em;
		text-transform: uppercase;
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
</style>
