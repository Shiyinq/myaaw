<script lang="ts">
	import { untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { authStore, toastStore } from '$lib/stores';
	import { getErrorMessage } from '$lib/utils/api';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import TopBarButton from '$lib/components/app/TopBarButton.svelte';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import {
		Card,
		CardContent,
		CardDescription,
		CardHeader,
		CardTitle
	} from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import {
		Eye,
		EyeOff,
		KeyRound,
		Loader2,
		LogOut,
		Pencil,
		ShieldCheck,
		UserRound
	} from 'lucide-svelte';

	const { t } = useTranslation();

	let passwordDialogOpen = $state(false);
	let currentPassword = $state('');
	let newPassword = $state('');
	let confirmPassword = $state('');
	let showCurrentPassword = $state(false);
	let showNewPassword = $state(false);
	let showConfirmPassword = $state(false);
	let passwordLoading = $state(false);
	let passwordError = $state('');

	let usernameDialogOpen = $state(false);
	let usernamePassword = $state('');
	let newUsername = $state('');
	let showUsernamePassword = $state(false);
	let usernameLoading = $state(false);
	let usernameError = $state('');

	const forcePasswordChange = $derived(page.url.searchParams.get('force') === '1');

	const initials = $derived.by(() => {
		const name = (authStore.profile?.username ?? '').trim();
		if (!name) return 'MY';
		return name.slice(0, 2).toUpperCase();
	});

	const canSubmitPassword = $derived(
		currentPassword.length > 0 &&
			newPassword.length > 0 &&
			confirmPassword.length > 0 &&
			!passwordLoading
	);

	const canSubmitUsername = $derived(
		newUsername.trim().length > 0 && usernamePassword.length > 0 && !usernameLoading
	);

	let forceOpened = false;
	$effect(() => {
		if (!forcePasswordChange || forceOpened) return;
		forceOpened = true;
		passwordDialogOpen = true;
	});

	$effect(() => {
		if (!passwordDialogOpen) return;
		untrack(() => {
			currentPassword = '';
			newPassword = '';
			confirmPassword = '';
			showCurrentPassword = false;
			showNewPassword = false;
			showConfirmPassword = false;
			passwordError = '';
			passwordLoading = false;
		});
	});

	$effect(() => {
		if (!usernameDialogOpen) return;
		untrack(() => {
			newUsername = authStore.profile?.username ?? '';
			usernamePassword = '';
			showUsernamePassword = false;
			usernameError = '';
			usernameLoading = false;
		});
	});

	async function changePassword(event: SubmitEvent) {
		event.preventDefault();
		passwordError = '';
		if (newPassword.length < 8) {
			passwordError = t('myaaw.profile.password_too_short');
			return;
		}
		if (newPassword !== confirmPassword) {
			passwordError = t('myaaw.profile.password_mismatch');
			return;
		}
		passwordLoading = true;
		try {
			await authStore.changePassword({
				current_password: currentPassword,
				new_password: newPassword
			});
			passwordDialogOpen = false;
			toastStore.success(t('myaaw.profile.password_updated'));
			await goto('/login');
		} catch (err) {
			passwordError = getErrorMessage(err);
		} finally {
			passwordLoading = false;
		}
	}

	async function changeUsername(event: SubmitEvent) {
		event.preventDefault();
		usernameError = '';
		if (!newUsername.trim()) {
			usernameError = t('myaaw.profile.username_required');
			return;
		}
		usernameLoading = true;
		try {
			await authStore.changeUsername({
				password: usernamePassword,
				new_username: newUsername.trim()
			});
			usernameDialogOpen = false;
			toastStore.success(t('myaaw.profile.username_updated'));
			usernamePassword = '';
			newUsername = '';
		} catch (err) {
			usernameError = getErrorMessage(err);
		} finally {
			usernameLoading = false;
		}
	}

	async function logout() {
		await authStore.logout();
		await goto('/login');
	}
</script>

<TopBarActions id="profile-logout">
	{#snippet children()}
		<TopBarButton label={t('common.logout')} icon={LogOut} onclick={logout} />
	{/snippet}
</TopBarActions>

<div class="mx-auto w-full max-w-3xl space-y-4">
	{#if authStore.profileError}
		<div
			role="alert"
			class="rounded-xl border border-destructive/30 bg-destructive/10 px-3.5 py-2.5 text-sm text-destructive"
		>
			{authStore.profileError}
		</div>
	{/if}
	{#if forcePasswordChange}
		<div
			role="alert"
			class="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-destructive/40 bg-destructive/10 px-3.5 py-2.5 text-sm text-destructive"
		>
			<span>{t('myaaw.profile.force_password')}</span>
			<Button size="sm" variant="destructive" onclick={() => (passwordDialogOpen = true)}>
				{t('myaaw.profile.password')}
			</Button>
		</div>
	{/if}

	<Card>
		<CardHeader>
			<CardTitle class="flex items-center gap-2">
				<UserRound class="h-4 w-4 text-primary" />
				{t('myaaw.profile.title')}
			</CardTitle>
			<CardDescription>{t('myaaw.profile.credentials_note')}</CardDescription>
		</CardHeader>
		<CardContent>
			<div class="flex flex-col gap-4 sm:flex-row sm:items-center">
				<div
					class="grid h-12 w-12 shrink-0 place-items-center rounded-full border bg-muted text-sm font-semibold tracking-tight text-foreground"
					aria-hidden="true"
				>
					{initials}
				</div>
				<div class="grid min-w-0 flex-1 gap-3 sm:grid-cols-3">
					<div class="min-w-0 space-y-0.5">
						<p class="text-[11px] font-medium uppercase tracking-widest text-muted-foreground">
							{t('common.username')}
						</p>
						<p class="truncate text-sm font-semibold text-foreground">
							{authStore.profile?.username ?? '—'}
						</p>
					</div>
					<div class="min-w-0 space-y-1">
						<p class="text-[11px] font-medium uppercase tracking-widest text-muted-foreground">
							{t('myaaw.profile.role')}
						</p>
						<div>
							{#if authStore.profile?.role}
								<Badge variant="secondary" class="capitalize">{authStore.profile.role}</Badge>
							{:else}
								<span class="text-sm text-muted-foreground">—</span>
							{/if}
						</div>
					</div>
					<div class="min-w-0 space-y-0.5">
						<p class="text-[11px] font-medium uppercase tracking-widest text-muted-foreground">
							user_id
						</p>
						<p class="truncate font-mono text-xs text-muted-foreground">
							{authStore.profile?.user_id ?? '—'}
						</p>
					</div>
				</div>
			</div>
		</CardContent>
	</Card>

	<Card>
		<CardHeader>
			<CardTitle class="flex items-center gap-2">
				<ShieldCheck class="h-4 w-4 text-primary" />
				{t('myaaw.profile.security')}
			</CardTitle>
			<CardDescription>{t('myaaw.profile.security_desc')}</CardDescription>
		</CardHeader>
		<CardContent class="space-y-3">
			<div class="divide-y rounded-xl border bg-background/40">
				<div class="flex flex-col gap-3 p-3.5 sm:flex-row sm:items-center sm:justify-between">
					<div class="flex min-w-0 items-start gap-3">
						<span
							class="mt-0.5 grid h-8 w-8 shrink-0 place-items-center rounded-lg border bg-card text-muted-foreground"
						>
							<UserRound class="h-4 w-4" />
						</span>
						<div class="min-w-0 space-y-0.5">
							<p class="text-sm font-medium text-foreground">{t('myaaw.profile.username')}</p>
							<p class="text-xs text-muted-foreground">{t('myaaw.profile.username_desc')}</p>
						</div>
					</div>
					<Button
						type="button"
						variant="outline"
						size="sm"
						data-open-dialog="username"
						class="w-fit shrink-0 gap-1.5"
						onclick={() => (usernameDialogOpen = true)}
					>
						<Pencil class="h-3.5 w-3.5" />
						{t('common.edit')}
					</Button>
				</div>

				<div class="flex flex-col gap-3 p-3.5 sm:flex-row sm:items-center sm:justify-between">
					<div class="flex min-w-0 items-start gap-3">
						<span
							class="mt-0.5 grid h-8 w-8 shrink-0 place-items-center rounded-lg border bg-card text-muted-foreground"
						>
							<KeyRound class="h-4 w-4" />
						</span>
						<div class="min-w-0 space-y-0.5">
							<p class="text-sm font-medium text-foreground">{t('myaaw.profile.password')}</p>
							<p class="text-xs text-muted-foreground">{t('myaaw.profile.password_desc')}</p>
						</div>
					</div>
					<Button
						type="button"
						variant={forcePasswordChange ? 'default' : 'outline'}
						size="sm"
						data-open-dialog="password"
						class="w-fit shrink-0 gap-1.5"
						onclick={() => (passwordDialogOpen = true)}
					>
						<KeyRound class="h-3.5 w-3.5" />
						{t('myaaw.profile.password')}
					</Button>
				</div>
			</div>

			<p class="text-xs text-muted-foreground">{t('myaaw.profile.session_note')}</p>
		</CardContent>
	</Card>
</div>

<!-- Change password modal -->
<Dialog.Root bind:open={passwordDialogOpen}>
	<Dialog.Content
		class="inset-0 top-0 left-0 flex h-dvh max-h-dvh w-screen max-w-none translate-x-0 translate-y-0 flex-col gap-0 overflow-hidden rounded-none bg-background p-0 ring-0 sm:inset-auto sm:top-1/2 sm:left-1/2 sm:h-auto sm:max-h-[90vh] sm:w-full sm:max-w-md sm:-translate-x-1/2 sm:-translate-y-1/2 sm:rounded-xl sm:bg-popover sm:ring-1"
	>
		<Dialog.Header
			class="shrink-0 border-b px-4 py-3.5 pr-12 text-left sm:border-b-0 sm:px-5 sm:pt-5 sm:pb-2"
		>
			<Dialog.Title class="flex items-center gap-2">
				<ShieldCheck class="h-4 w-4 text-primary" />
				{t('myaaw.profile.password')}
			</Dialog.Title>
			<Dialog.Description>{t('myaaw.profile.password_desc')}</Dialog.Description>
		</Dialog.Header>

		<form
			onsubmit={changePassword}
			autocomplete="off"
			class="flex min-h-0 flex-1 flex-col overflow-hidden"
		>
			<div
				class="min-h-0 flex-1 space-y-4 overflow-y-auto overscroll-contain px-4 py-4 sm:px-5 sm:py-2"
			>
				<div class="space-y-1.5">
					<Label for="current-password">{t('myaaw.profile.current_password')}</Label>
					<div class="relative">
						<Input
							id="current-password"
							type={showCurrentPassword ? 'text' : 'password'}
							autocomplete="current-password"
							bind:value={currentPassword}
							required
							class="pr-9"
						/>
						<button
							type="button"
							tabindex={-1}
							aria-label={t('myaaw.profile.current_password')}
							onclick={() => (showCurrentPassword = !showCurrentPassword)}
							class="absolute top-1/2 right-2.5 -translate-y-1/2 text-muted-foreground transition-colors hover:text-foreground"
						>
							{#if showCurrentPassword}
								<EyeOff class="h-4 w-4" />
							{:else}
								<Eye class="h-4 w-4" />
							{/if}
						</button>
					</div>
				</div>

				<div class="space-y-1.5">
					<Label for="new-password">{t('myaaw.profile.new_password')}</Label>
					<div class="relative">
						<Input
							id="new-password"
							type={showNewPassword ? 'text' : 'password'}
							autocomplete="new-password"
							bind:value={newPassword}
							required
							class="pr-9"
						/>
						<button
							type="button"
							tabindex={-1}
							aria-label={t('myaaw.profile.new_password')}
							onclick={() => (showNewPassword = !showNewPassword)}
							class="absolute top-1/2 right-2.5 -translate-y-1/2 text-muted-foreground transition-colors hover:text-foreground"
						>
							{#if showNewPassword}
								<EyeOff class="h-4 w-4" />
							{:else}
								<Eye class="h-4 w-4" />
							{/if}
						</button>
					</div>
					<p class="text-[11px] text-muted-foreground">{t('myaaw.profile.password_hint')}</p>
				</div>

				<div class="space-y-1.5">
					<Label for="confirm-password">{t('myaaw.profile.confirm_password')}</Label>
					<div class="relative">
						<Input
							id="confirm-password"
							type={showConfirmPassword ? 'text' : 'password'}
							autocomplete="new-password"
							bind:value={confirmPassword}
							required
							class="pr-9"
						/>
						<button
							type="button"
							tabindex={-1}
							aria-label={t('myaaw.profile.confirm_password')}
							onclick={() => (showConfirmPassword = !showConfirmPassword)}
							class="absolute top-1/2 right-2.5 -translate-y-1/2 text-muted-foreground transition-colors hover:text-foreground"
						>
							{#if showConfirmPassword}
								<EyeOff class="h-4 w-4" />
							{:else}
								<Eye class="h-4 w-4" />
							{/if}
						</button>
					</div>
				</div>

				{#if passwordError}
					<p
						role="alert"
						class="rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 text-xs text-destructive"
					>
						{passwordError}
					</p>
				{/if}
			</div>

			<Dialog.Footer
				class="mx-0 mb-0 shrink-0 flex-row justify-end gap-2 rounded-none border-t bg-muted/40 px-4 py-3.5 sm:rounded-b-xl sm:px-5 sm:py-3.5"
			>
				<Button
					type="button"
					variant="outline"
					onclick={() => (passwordDialogOpen = false)}
					disabled={passwordLoading}
				>
					{t('common.cancel')}
				</Button>
				<Button type="submit" disabled={!canSubmitPassword}>
					{#if passwordLoading}
						<Loader2 class="mr-1.5 h-3.5 w-3.5 animate-spin" />
					{/if}
					{t('common.save')}
				</Button>
			</Dialog.Footer>
		</form>
	</Dialog.Content>
</Dialog.Root>

<!-- Change username modal -->
<Dialog.Root bind:open={usernameDialogOpen}>
	<Dialog.Content
		class="inset-0 top-0 left-0 flex h-dvh max-h-dvh w-screen max-w-none translate-x-0 translate-y-0 flex-col gap-0 overflow-hidden rounded-none bg-background p-0 ring-0 sm:inset-auto sm:top-1/2 sm:left-1/2 sm:h-auto sm:max-h-[90vh] sm:w-full sm:max-w-md sm:-translate-x-1/2 sm:-translate-y-1/2 sm:rounded-xl sm:bg-popover sm:ring-1"
	>
		<Dialog.Header
			class="shrink-0 border-b px-4 py-3.5 pr-12 text-left sm:border-b-0 sm:px-5 sm:pt-5 sm:pb-2"
		>
			<Dialog.Title class="flex items-center gap-2">
				<UserRound class="h-4 w-4 text-primary" />
				{t('myaaw.profile.username')}
			</Dialog.Title>
			<Dialog.Description>{t('myaaw.profile.username_desc')}</Dialog.Description>
		</Dialog.Header>

		<form
			onsubmit={changeUsername}
			autocomplete="off"
			class="flex min-h-0 flex-1 flex-col overflow-hidden"
		>
			<div
				class="min-h-0 flex-1 space-y-4 overflow-y-auto overscroll-contain px-4 py-4 sm:px-5 sm:py-2"
			>
				<div class="space-y-1.5">
					<Label for="new-username">{t('myaaw.profile.new_username')}</Label>
					<Input id="new-username" autocomplete="username" bind:value={newUsername} required />
				</div>

				<div class="space-y-1.5">
					<Label for="username-password">{t('common.password')}</Label>
					<div class="relative">
						<Input
							id="username-password"
							type={showUsernamePassword ? 'text' : 'password'}
							autocomplete="current-password"
							bind:value={usernamePassword}
							required
							class="pr-9"
						/>
						<button
							type="button"
							tabindex={-1}
							aria-label={t('common.password')}
							onclick={() => (showUsernamePassword = !showUsernamePassword)}
							class="absolute top-1/2 right-2.5 -translate-y-1/2 text-muted-foreground transition-colors hover:text-foreground"
						>
							{#if showUsernamePassword}
								<EyeOff class="h-4 w-4" />
							{:else}
								<Eye class="h-4 w-4" />
							{/if}
						</button>
					</div>
				</div>

				{#if usernameError}
					<p
						role="alert"
						class="rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 text-xs text-destructive"
					>
						{usernameError}
					</p>
				{/if}
			</div>

			<Dialog.Footer
				class="mx-0 mb-0 shrink-0 flex-row justify-end gap-2 rounded-none border-t bg-muted/40 px-4 py-3.5 sm:rounded-b-xl sm:px-5 sm:py-3.5"
			>
				<Button
					type="button"
					variant="outline"
					onclick={() => (usernameDialogOpen = false)}
					disabled={usernameLoading}
				>
					{t('common.cancel')}
				</Button>
				<Button type="submit" disabled={!canSubmitUsername}>
					{#if usernameLoading}
						<Loader2 class="mr-1.5 h-3.5 w-3.5 animate-spin" />
					{/if}
					{t('common.save')}
				</Button>
			</Dialog.Footer>
		</form>
	</Dialog.Content>
</Dialog.Root>
