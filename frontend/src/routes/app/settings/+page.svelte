<script lang="ts">
	import { goto } from '$app/navigation';
	import { authStore, toastStore } from '$lib/stores';
	import { getErrorMessage } from '$lib/utils/api';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import TopBarActions from '$lib/components/app/TopBarActions.svelte';
	import TopBarButton from '$lib/components/app/TopBarButton.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Separator } from '$lib/components/ui/separator';
	import { LogOut, Palette, ShieldCheck, User } from 'lucide-svelte';
	import { userPrefersMode, setMode, resetMode } from 'mode-watcher';
	import type { Locale } from '$lib/i18n';

	const { t, locale, changeLocale, availableLocales } = useTranslation();

	let currentPassword = $state('');
	let newPassword = $state('');
	let confirmPassword = $state('');
	let passwordLoading = $state(false);
	let passwordError = $state('');

	let usernamePassword = $state('');
	let newUsername = $state('');
	let usernameLoading = $state(false);
	let usernameError = $state('');

	async function changePassword() {
		passwordError = '';
		if (newPassword.length < 8) {
			passwordError = t('myaaw.settings.password_too_short');
			return;
		}
		if (newPassword !== confirmPassword) {
			passwordError = t('myaaw.settings.password_mismatch');
			return;
		}
		passwordLoading = true;
		try {
			await authStore.changePassword({
				current_password: currentPassword,
				new_password: newPassword
			});
			toastStore.success(t('myaaw.settings.password_updated'));
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
			usernameError = t('myaaw.settings.username_required');
			return;
		}
		usernameLoading = true;
		try {
			await authStore.changeUsername({
				password: usernamePassword,
				new_username: newUsername.trim()
			});
			toastStore.success(t('myaaw.settings.username_updated'));
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

	function changeLanguage(event: Event) {
		changeLocale((event.currentTarget as HTMLSelectElement).value as Locale);
	}
</script>

<TopBarActions id="settings-logout">
	{#snippet children()}
		<TopBarButton label={t('common.logout')} icon={LogOut} onclick={logout} />
	{/snippet}
</TopBarActions>

<div class="space-y-4">
	<Card>
		<CardHeader>
			<CardTitle class="flex items-center gap-2">
				<User class="h-4 w-4" />
				{t('myaaw.settings.profile')}
			</CardTitle>
			<CardDescription>{t('myaaw.settings.profile_desc')}</CardDescription>
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

	<Card>
		<CardHeader>
			<CardTitle class="flex items-center gap-2">
				<Palette class="h-4 w-4" />
				{t('myaaw.settings.app_settings')}
			</CardTitle>
			<CardDescription>{t('myaaw.settings.app_settings_desc')}</CardDescription>
		</CardHeader>
		<CardContent class="grid gap-4 sm:grid-cols-2">
			<div class="grid gap-1">
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
			<div class="grid gap-1">
				<Label>{t('myaaw.settings.theme')}</Label>
				<div class="flex gap-2">
					<Button
						variant={userPrefersMode.current === 'light' ? 'default' : 'outline'}
						size="sm"
						class="flex-1"
						onclick={() => setMode('light')}
					>
						{t('myaaw.settings.theme_light')}
					</Button>
					<Button
						variant={userPrefersMode.current === 'dark' ? 'default' : 'outline'}
						size="sm"
						class="flex-1"
						onclick={() => setMode('dark')}
					>
						{t('myaaw.settings.theme_dark')}
					</Button>
					<Button
						variant={userPrefersMode.current === 'system' ? 'default' : 'outline'}
						size="sm"
						class="flex-1"
						onclick={() => resetMode()}
					>
						{t('myaaw.settings.theme_system')}
					</Button>
				</div>
			</div>
		</CardContent>
	</Card>

	<div class="grid gap-4 md:grid-cols-2">
		<Card>
			<CardHeader>
				<CardTitle class="flex items-center gap-2">
					<ShieldCheck class="h-4 w-4" />
					{t('myaaw.settings.password')}
				</CardTitle>
				<CardDescription>{t('myaaw.settings.password_desc')}</CardDescription>
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
						<Label for="current-password">{t('myaaw.settings.current_password')}</Label>
						<Input id="current-password" type="password" bind:value={currentPassword} required />
					</div>
					<div class="grid gap-1">
						<Label for="new-password">{t('myaaw.settings.new_password')}</Label>
						<Input id="new-password" type="password" bind:value={newPassword} required />
					</div>
					<div class="grid gap-1">
						<Label for="confirm-password">{t('myaaw.settings.confirm_password')}</Label>
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
				<CardTitle>{t('myaaw.settings.username')}</CardTitle>
				<CardDescription>{t('myaaw.settings.username_desc')}</CardDescription>
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
						<Label for="new-username">{t('myaaw.settings.new_username')}</Label>
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
	<p class="text-xs text-muted-foreground">{t('myaaw.settings.session_note')}</p>
</div>
