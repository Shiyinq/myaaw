<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { Keyboard, LogOut, Settings2 } from 'lucide-svelte';
	import { Button } from '$lib/components/ui/button';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { authStore, shortcutHelp } from '$lib/stores';
	import { useTranslation } from '$lib/i18n/useTranslation';

	interface Props {
		/** Icon-only trigger, for the collapsed sidebar and the focus rail. */
		compact?: boolean;
		/** Which way the menu opens when the trigger is compact. */
		align?: 'start' | 'center' | 'end';
		/** Which side of the trigger the menu opens on; the focus tab bar opens upward. */
		side?: 'top' | 'bottom';
		/** Bottom-bar tab styling: icon over label, like a phone app tab. */
		tab?: boolean;
	}

	let { compact = false, align = 'end', side = 'bottom', tab = false }: Props = $props();

	const { t } = useTranslation();
	const initials = $derived((authStore.profile?.username ?? 'MY').slice(0, 2).toUpperCase());
	const profileActive = $derived(page.url.pathname.startsWith('/app/profile'));

	async function logout() {
		await authStore.logout();
		await goto('/login');
	}
</script>

<DropdownMenu.Root>
	<DropdownMenu.Trigger>
		{#snippet child({ props })}
			{#if tab}
				<button
					{...props}
					class="flex h-full flex-1 flex-col items-center justify-center gap-0.5 px-1 py-2"
					aria-label={t('myaaw.nav.profile')}
				>
					<span
						class={[
							'flex h-7 w-9 items-center justify-center rounded-full text-[10px] font-bold uppercase transition-colors',
							profileActive ? 'bg-primary/10 text-primary' : 'text-muted-foreground'
						]}
					>
						{initials}
					</span>
					<span
						class={[
							'text-[10px] font-medium transition-colors',
							profileActive ? 'text-primary' : 'text-muted-foreground'
						]}
					>
						{t('myaaw.nav.profile')}
					</span>
				</button>
			{:else if compact}
				<Button
					{...props}
					variant="outline"
					size="icon-sm"
					class="rounded-full"
					aria-label={t('myaaw.nav.profile')}
				>
					<span class="text-[10px] font-bold uppercase">{initials}</span>
				</Button>
			{:else}
				<Button {...props} variant="outline" class="h-auto w-full justify-start gap-2 py-2">
					<span
						class="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-muted text-[10px] font-bold uppercase"
					>
						{initials}
					</span>
					<span class="flex min-w-0 flex-col items-start leading-tight">
						<span class="w-full truncate text-sm font-medium">
							{authStore.profile?.username ?? '—'}
						</span>
						<span class="text-[10px] text-muted-foreground">
							user #{authStore.profile?.user_id ?? '—'}
						</span>
					</span>
				</Button>
			{/if}
		{/snippet}
	</DropdownMenu.Trigger>
	<DropdownMenu.Content align={compact || tab ? align : 'start'} {side} class="w-56">
		<DropdownMenu.Item class="flex-col items-start gap-0 p-2" onclick={() => goto('/app/profile')}>
			<span class="text-sm font-medium">{authStore.profile?.username ?? '—'}</span>
			<span class="text-xs text-muted-foreground">
				user #{authStore.profile?.user_id ?? '—'}
			</span>
		</DropdownMenu.Item>
		<DropdownMenu.Separator />
		<DropdownMenu.Item onclick={() => goto('/app/settings')}>
			<Settings2 class="mr-2 h-4 w-4" />
			{t('myaaw.nav.settings')}
		</DropdownMenu.Item>
		<DropdownMenu.Item onclick={() => shortcutHelp.show()}>
			<Keyboard class="mr-2 h-4 w-4" />
			{t('myaaw.shortcuts.title')}
		</DropdownMenu.Item>
		<DropdownMenu.Separator />
		<DropdownMenu.Item variant="destructive" onclick={logout}>
			<LogOut class="mr-2 h-4 w-4" />
			{t('common.logout')}
		</DropdownMenu.Item>
	</DropdownMenu.Content>
</DropdownMenu.Root>
