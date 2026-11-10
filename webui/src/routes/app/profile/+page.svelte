<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { authStore, toastStore } from '$lib/stores';
	import { getErrorMessage } from '$lib/utils/api';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import TopBarButton from '$lib/components/app/TopBarButton.svelte';
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
	import { Separator } from '$lib/components/ui/separator';
	import { LogOut, ShieldCheck, User } from 'lucide-svelte';

	const { t } = useTranslation();

	let currentPassword = $state('');
	let newPassword = $state('');
	let confirmPassword = $state('');
	let passwordLoading = $state(false);
	let passwordError = $state('');

	let usernamePassword = $state('');
	let newUsername = $state('');
	let usernameLoading = $state(false);
	let usernameError = $state('');

	const forcePasswordChange = $derived(page.url.searchParams.get('force') === '1');

	async function changePassword() {
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
			toastStore.success(t('myaaw.profile.password_updated'));
			await goto('/login');
		} catch (err) {
			passwordError = getErrorMessage(err);
		} finally {
			passwordLoading = false;
		}
	}

	async function changeUsername() {
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

<div class="space-y-4">
	{#if forcePasswordChange}
		<div
			class="rounded-md border border-destructive/40 bg-destructive/10 p-3 text-sm text-destructive"
		>
			{t('myaaw.profile.force_password')}
		</div>
	{/if}
	<Card>
		<CardHeader>
			<CardTitle class="flex items-center gap-2">
				<User class="h-4 w-4" />
				{t('myaaw.profile.title')}
			</CardTitle>
			<CardDescription>{t('myaaw.profile.credentials_note')}</CardDescription>
		</CardHeader>
		<CardContent class="grid gap-3 sm:grid-cols-2">
			<div class="space-y-1">
				<p class="text-xs uppercase tracking-widest text-muted-foreground">
					{t('common.username')}
				</p>
				<p class="font-medium">{authStore.profile?.username ?? '—'}</p>
			</div>
			<div class="space-y-1">
				<p class="text-xs uppercase tracking-widest text-muted-foreground">user_id</p>
				<p class="font-mono text-sm">{authStore.profile?.user_id ?? '—'}</p>
			</div>
		</CardContent>
	</Card>

	<div class="grid gap-4 md:grid-cols-2">
		<Card>
			<CardHeader>
				<CardTitle class="flex items-center gap-2">
					<ShieldCheck class="h-4 w-4" />
					{t('myaaw.profile.password')}
				</CardTitle>
				<CardDescription>{t('myaaw.profile.password_desc')}</CardDescription>
			</CardHeader>
			<CardContent>
				<form
					class="space-y-3"
					onsubmit={(event) => {
						event.preventDefault();
						void changePassword();
					}}
				>
					{#if passwordError}
						<p class="rounded-md bg-destructive/10 p-2 text-xs text-destructive">{passwordError}</p>
					{/if}
					<div class="grid gap-1">
						<Label for="current-password">{t('myaaw.profile.current_password')}</Label>
						<Input id="current-password" type="password" bind:value={currentPassword} required />
					</div>
					<div class="grid gap-1">
						<Label for="new-password">{t('myaaw.profile.new_password')}</Label>
						<Input id="new-password" type="password" bind:value={newPassword} required />
					</div>
					<div class="grid gap-1">
						<Label for="confirm-password">{t('myaaw.profile.confirm_password')}</Label>
						<Input id="confirm-password" type="password" bind:value={confirmPassword} required />
					</div>
					<Button type="submit" size="sm" loading={passwordLoading}>
						{t('common.save')}
					</Button>
				</form>
			</CardContent>
		</Card>

		<Card>
			<CardHeader>
				<CardTitle>{t('myaaw.profile.username')}</CardTitle>
				<CardDescription>{t('myaaw.profile.username_desc')}</CardDescription>
			</CardHeader>
			<CardContent>
				<form
					class="space-y-3"
					onsubmit={(event) => {
						event.preventDefault();
						void changeUsername();
					}}
				>
					{#if usernameError}
						<p class="rounded-md bg-destructive/10 p-2 text-xs text-destructive">{usernameError}</p>
					{/if}
					<div class="grid gap-1">
						<Label for="new-username">{t('myaaw.profile.new_username')}</Label>
						<Input id="new-username" bind:value={newUsername} required />
					</div>
					<div class="grid gap-1">
						<Label for="username-password">{t('common.password')}</Label>
						<Input id="username-password" type="password" bind:value={usernamePassword} required />
					</div>
					<Button type="submit" size="sm" loading={usernameLoading}>
						{t('common.save')}
					</Button>
				</form>
			</CardContent>
		</Card>
	</div>

	<Separator />
	<p class="text-xs text-muted-foreground">{t('myaaw.profile.session_note')}</p>
</div>
