<script lang="ts">
	import { onMount } from 'svelte';
	import SEO from '$lib/components/common/SEO.svelte';
	import CapabilityMarquee from '$lib/components/landing/CapabilityMarquee.svelte';
	import DoorsSection from '$lib/components/landing/DoorsSection.svelte';
	import FleetSection from '$lib/components/landing/FleetSection.svelte';
	import GsapControlPlaneOrb from '$lib/components/landing/GsapControlPlaneOrb.svelte';
	import LandingFooter from '$lib/components/landing/LandingFooter.svelte';
	import LandingHeader from '$lib/components/landing/LandingHeader.svelte';
	import LandingHero from '$lib/components/landing/LandingHero.svelte';
	import RulesInstallSection from '$lib/components/landing/RulesInstallSection.svelte';
	import RuntimesSection from '$lib/components/landing/RuntimesSection.svelte';
	import ScrollTrail from '$lib/components/landing/ScrollTrail.svelte';
	import WorkspaceSection from '$lib/components/landing/WorkspaceSection.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { LANDING_SECTIONS } from '$lib/landing/sections';
	import { landingScroll } from '$lib/landing/smoothScroll.svelte';

	const { t } = useTranslation();

	onMount(() => {
		const cleanup = landingScroll.mount(LANDING_SECTIONS.map((section) => section.id));

		const hash = window.location.hash;
		if (hash && hash.length > 1) {
			requestAnimationFrame(() => {
				landingScroll.scrollTo(hash, { offset: -64, immediate: true });
			});
		}

		return () => {
			cleanup();
		};
	});
</script>

<SEO title={t('landing.meta.title')} description={t('landing.meta.description')} />

<LandingHeader />

<div id="main" class="relative pt-14">
	<LandingHero />
	<CapabilityMarquee />

	<section class="border-b border-border/60">
		<div class="mx-auto max-w-6xl px-4 py-12 sm:px-6 lg:py-16">
			<GsapControlPlaneOrb />
		</div>
	</section>

	<RuntimesSection />
	<WorkspaceSection />
	<FleetSection />
	<DoorsSection />
	<RulesInstallSection />
	<LandingFooter />
</div>

<ScrollTrail />
