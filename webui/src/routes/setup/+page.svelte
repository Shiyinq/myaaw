<script lang="ts">
	import { onMount } from 'svelte';
	import { Button } from '$lib/components/ui/button';
	import {
		Card,
		CardContent,
		CardDescription,
		CardHeader,
		CardTitle
	} from '$lib/components/ui/card';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import type { PageData } from './$types';

	interface Status {
		binaryInstalled: boolean;
		controlPlaneReachable: boolean;
		binaryPath: string;
	}

	interface InstallResponse {
		message?: string;
		detail?: string;
	}

	interface InstallProgress {
		phase: 'idle' | 'release' | 'download' | 'verify' | 'write' | 'start' | 'done' | 'error';
		percent: number;
		error?: string;
	}

	const PHASE_KEYS: Record<string, string> = {
		release: 'myaaw.setup.phase_release',
		download: 'myaaw.setup.phase_download',
		verify: 'myaaw.setup.phase_verify',
		write: 'myaaw.setup.phase_write',
		start: 'myaaw.setup.phase_start'
	};

	let { data }: { data: PageData } = $props();
	const { t } = useTranslation();

	// A null token means the server already found an installed binary.
	let installed = $state(false);
	let status = $state<Status | null>(null);
	let installing = $state(false);
	let progress = $state<InstallProgress | null>(null);
	let error = $state('');

	const binaryReady = $derived(
		data.token === null || installed || status?.binaryInstalled === true
	);

	async function refresh() {
		try {
			const response = await fetch('/setup/status');
			if (response.ok) {
				status = (await response.json()) as Status;
			}
		} catch {
			status = null;
		}
	}

	async function refreshProgress() {
		try {
			const response = await fetch('/setup/progress', { cache: 'no-store' });
			if (response.ok) {
				progress = (await response.json()) as InstallProgress;
			}
		} catch {
			// Keep the last value; the POST result still decides success.
		}
	}

	async function install() {
		installing = true;
		error = '';
		progress = { phase: 'release', percent: 0 };
		const timer = setInterval(() => void refreshProgress(), 400);
		try {
			const response = await fetch('/setup/install', {
				method: 'POST',
				headers: { 'x-setup-token': data.token ?? '' }
			});
			const payload = (await response.json().catch(() => ({}))) as InstallResponse;
			if (!response.ok) {
				throw new Error(
					payload.message || payload.detail || `${t('myaaw.setup.failed')} (${response.status})`
				);
			}
			installed = true;
			await refresh();
		} catch (cause) {
			error = cause instanceof Error ? cause.message : t('myaaw.setup.failed');
		} finally {
			clearInterval(timer);
			await refreshProgress();
			installing = false;
		}
	}

	onMount(() => {
		void refresh();
	});
</script>

<svelte:head>
	<title>{t('myaaw.setup.title')}</title>
</svelte:head>

<div class="min-h-screen flex items-center justify-center p-6 bg-background">
	<div class="w-full max-w-lg space-y-4">
		<Card>
			<CardHeader>
				<CardTitle>{t('myaaw.setup.title')}</CardTitle>
				<CardDescription>{t('myaaw.setup.subtitle')}</CardDescription>
			</CardHeader>
			<CardContent class="space-y-4">
				{#if status}
					<div class="space-y-1 text-sm">
						<p>
							{t('myaaw.setup.binary')}:
							{status.binaryInstalled
								? `${t('myaaw.setup.installed')} (${status.binaryPath})`
								: t('myaaw.setup.not_installed')}
						</p>
						<p>
							{t('myaaw.setup.control_plane')}:
							{status.controlPlaneReachable
								? t('myaaw.setup.reachable')
								: t('myaaw.setup.not_reachable')}
						</p>
					</div>
				{:else}
					<p class="text-sm text-muted-foreground">{t('myaaw.setup.checking')}</p>
				{/if}

				{#if binaryReady}
					<p class="text-sm text-green-600">{t('myaaw.setup.installed_note')}</p>
					<Button
						onclick={() => {
							window.location.href = '/login';
						}}
					>
						{t('myaaw.setup.continue')}
					</Button>
				{:else}
					<Button onclick={install} loading={installing} disabled={!data.token}>
						{installing ? t('myaaw.setup.installing') : t('myaaw.setup.install')}
					</Button>
					{#if installing}
						<div class="w-full space-y-1" aria-live="polite">
							<div class="h-2 w-full overflow-hidden rounded-full bg-muted">
								<div
									class="h-full rounded-full bg-primary transition-all"
									style="width: {progress?.percent ?? 0}%"
								></div>
							</div>
							<p class="text-xs text-muted-foreground">
								{t(PHASE_KEYS[progress?.phase ?? 'release'] ?? 'myaaw.setup.installing')}
								· {progress?.percent ?? 0}%
							</p>
						</div>
					{/if}
					{#if !data.token}
						<p class="text-xs text-muted-foreground">{t('myaaw.setup.no_token')}</p>
					{/if}
				{/if}

				{#if error}
					<p class="text-sm text-destructive">{error}</p>
				{/if}

				<p class="text-xs text-muted-foreground">
					{t('myaaw.setup.already_have')}
					<a class="underline" href="/login">{t('myaaw.setup.sign_in')}</a>
					{t('myaaw.setup.server_url_suffix')}
				</p>
			</CardContent>
		</Card>
	</div>
</div>
