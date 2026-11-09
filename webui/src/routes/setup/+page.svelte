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

	let { data }: { data: PageData } = $props();

	// A null token means the server already found an installed binary.
	let installed = $state(false);
	let status = $state<Status | null>(null);
	let installing = $state(false);
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

	async function install() {
		installing = true;
		error = '';
		try {
			const response = await fetch('/setup/install', {
				method: 'POST',
				headers: { 'x-setup-token': data.token ?? '' }
			});
			const payload = (await response.json().catch(() => ({}))) as InstallResponse;
			if (!response.ok) {
				throw new Error(payload.message || payload.detail || `Install failed (${response.status})`);
			}
			installed = true;
			await refresh();
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Install failed';
		} finally {
			installing = false;
		}
	}

	onMount(() => {
		void refresh();
	});
</script>

<svelte:head>
	<title>Myaaw setup</title>
</svelte:head>

<div class="min-h-screen flex items-center justify-center p-6 bg-background">
	<div class="w-full max-w-lg space-y-4">
		<Card>
			<CardHeader>
				<CardTitle>Myaaw setup</CardTitle>
				<CardDescription>
					Install the Myaaw binary on this host, or connect to a remote control plane.
				</CardDescription>
			</CardHeader>
			<CardContent class="space-y-4">
				{#if status}
					<div class="space-y-1 text-sm">
						<p>
							Binary:
							{status.binaryInstalled ? `installed (${status.binaryPath})` : 'not installed'}
						</p>
						<p>Control plane: {status.controlPlaneReachable ? 'reachable' : 'not reachable'}</p>
					</div>
				{:else}
					<p class="text-sm text-muted-foreground">Checking this host…</p>
				{/if}

				{#if binaryReady}
					<p class="text-sm text-green-600">
						Binary installed. The control plane may take a moment to start.
					</p>
					<Button
						onclick={() => {
							window.location.href = '/login';
						}}
					>
						Continue to sign in
					</Button>
				{:else}
					<Button onclick={install} loading={installing} disabled={!data.token}>
						{installing ? 'Installing…' : 'Download & install Myaaw'}
					</Button>
					{#if !data.token}
						<p class="text-xs text-muted-foreground">
							Reload this page to get a fresh setup token.
						</p>
					{/if}
				{/if}

				{#if error}
					<p class="text-sm text-destructive">{error}</p>
				{/if}

				<p class="text-xs text-muted-foreground">
					Already have a control plane? <a class="underline" href="/login">Sign in</a> and enter its Server
					URL.
				</p>
			</CardContent>
		</Card>
	</div>
</div>
