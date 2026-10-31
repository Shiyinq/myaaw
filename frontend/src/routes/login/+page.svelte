<script lang="ts">
	import { authStore } from '$lib/stores';
	import { goto } from '$app/navigation';
	import { fade } from 'svelte/transition';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import SEO from '$lib/components/common/SEO.svelte';
	import { Input } from '$lib/components/ui/input';
	import { Button } from '$lib/components/ui/button';
	import {
		Card,
		CardHeader,
		CardTitle,
		CardDescription,
		CardContent
	} from '$lib/components/ui/card';
	import { Label } from '$lib/components/ui/label';
	import { logger } from '$lib/utils/logger';

	const { t } = useTranslation();

	let username = $state('');
	let password = $state('');

	let loading = $derived(authStore.isLoading);
	let error = $derived(authStore.error);

	async function handleLogin() {
		if (loading) return;
		try {
			await authStore.login({ username, password });
			await goto('/app');
		} catch (err) {
			logger.error('Login failed', err, { context: 'login' });
		}
	}

	// Enter submits directly instead of relying on the form's implicit submission,
	// which browser extensions and password-manager popups are allowed to swallow.
	function submitOnEnter(event: KeyboardEvent) {
		if (event.key !== 'Enter' || event.isComposing) return;
		event.preventDefault();
		void handleLogin();
	}
</script>

<SEO title="Myaaw | {t('common.login')}" description={t('myaaw.login.subtitle')} />

<div class="min-h-screen flex items-center justify-center p-6 bg-background relative">
	<div class="w-full max-w-md" in:fade={{ duration: 200 }}>
		<Card class="border-border">
			<CardHeader class="space-y-2 text-center pb-8">
				<div class="mx-auto mb-2 flex h-12 w-12 items-center justify-center rounded-xl bg-primary">
					<span class="text-xl font-bold text-primary-foreground">M</span>
				</div>
				<CardTitle class="text-3xl font-bold tracking-tight">Myaaw</CardTitle>
				<CardDescription>{t('myaaw.login.subtitle')}</CardDescription>
			</CardHeader>
			<CardContent>
				<form
					class="space-y-4"
					onsubmit={(event) => {
						event.preventDefault();
						void handleLogin();
					}}
				>
					{#if error}
						<div
							class="bg-destructive/15 text-destructive text-sm p-3 rounded-md flex items-center gap-2"
							transition:fade
						>
							<span
								class="w-5 h-5 flex items-center justify-center rounded-full bg-destructive text-destructive-foreground font-bold text-xs"
								>!</span
							>
							{error}
						</div>
					{/if}

					<div class="grid gap-2">
						<Label for="username">{t('common.username')}</Label>
						<Input
							id="username"
							name="username"
							type="text"
							autocomplete="username"
							placeholder="myaaw"
							bind:value={username}
							onkeydown={submitOnEnter}
							required
						/>
					</div>

					<div class="grid gap-2">
						<Label for="password">{t('common.password')}</Label>
						<Input
							id="password"
							name="password"
							type="password"
							autocomplete="current-password"
							placeholder="••••••••"
							bind:value={password}
							onkeydown={submitOnEnter}
							required
						/>
					</div>

					<Button type="submit" class="w-full mt-4" {loading}>
						{#if loading}
							{t('myaaw.login.submitting')}
						{:else}
							{t('myaaw.login.submit')}
						{/if}
					</Button>
				</form>
			</CardContent>
		</Card>
		<p class="mt-4 text-center text-xs text-muted-foreground">{t('myaaw.login.hint')}</p>
	</div>
</div>
