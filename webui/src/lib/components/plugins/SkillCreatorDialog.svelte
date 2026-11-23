<script lang="ts">
	import * as Dialog from '$lib/components/ui/dialog';
	import { Button } from '$lib/components/ui/button';
	import CopyButton from '$lib/components/common/CopyButton.svelte';
	import { Check } from 'lucide-svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';

	interface Props {
		open: boolean;
		/** Whether skill-creator is installed *and* switched on. */
		creatorEnabled: boolean;
		enabling?: boolean;
		onenable: () => void;
	}

	let { open = $bindable(), creatorEnabled, enabling = false, onenable }: Props = $props();

	const { t } = useTranslation();
	const prompt = $derived(t('myaaw.plugins.skill_dialog_prompt'));
</script>

<!--
	The dashboard cannot author a skill by itself: `skill-creator` is the skill that
	writes skills, so the dialog explains the requirement, offers the one-click
	enable, and hands over a prompt the user can paste into a chat.
-->
<Dialog.Root bind:open>
	<Dialog.Content
		class="inset-0 top-0 left-0 flex h-dvh max-h-dvh w-screen max-w-none translate-x-0 translate-y-0 flex-col gap-0 overflow-hidden rounded-none bg-background p-0 ring-0 sm:inset-auto sm:top-1/2 sm:left-1/2 sm:h-auto sm:max-h-[90vh] sm:w-full sm:max-w-lg sm:-translate-x-1/2 sm:-translate-y-1/2 sm:rounded-xl sm:bg-popover sm:ring-1"
	>
		<Dialog.Header
			class="shrink-0 border-b px-4 py-3.5 pr-12 text-left sm:border-b-0 sm:px-5 sm:pt-5 sm:pb-2"
		>
			<Dialog.Title>{t('myaaw.plugins.skill_dialog_title')}</Dialog.Title>
			<Dialog.Description>{t('myaaw.plugins.skill_dialog_body')}</Dialog.Description>
		</Dialog.Header>

		<div
			class="min-h-0 flex-1 space-y-4 overflow-y-auto overscroll-contain px-4 py-4 sm:px-5 sm:py-2"
		>
			<div class="flex flex-col gap-2 rounded-xl border bg-muted/40 p-3">
				{#if creatorEnabled}
					<p
						class="flex items-center gap-1.5 text-xs font-medium text-emerald-600 dark:text-emerald-400"
					>
						<Check class="h-3.5 w-3.5 shrink-0" />
						{t('myaaw.plugins.skill_dialog_enabled')}
					</p>
				{:else}
					<p class="text-xs text-muted-foreground">
						{t('myaaw.plugins.skill_dialog_requirement')}
					</p>
					<Button
						size="sm"
						variant="outline"
						class="self-start"
						loading={enabling}
						onclick={onenable}
					>
						{t('myaaw.plugins.skill_dialog_enable')}
					</Button>
				{/if}
			</div>

			<div class="space-y-1.5">
				<div class="flex items-center justify-between gap-2">
					<p class="text-[10px] font-semibold tracking-widest text-muted-foreground uppercase">
						{t('myaaw.plugins.skill_dialog_prompt_label')}
					</p>
					<CopyButton text={prompt} label />
				</div>
				<p
					class="rounded-xl border bg-background p-3 font-mono text-xs leading-relaxed whitespace-pre-wrap"
				>
					{prompt}
				</p>
			</div>
		</div>

		<Dialog.Footer
			class="mx-0 mb-0 shrink-0 flex-row justify-end gap-2 rounded-none border-t bg-muted/40 px-4 py-3.5 sm:rounded-b-xl sm:px-5 sm:py-3.5"
		>
			<Button variant="outline" onclick={() => (open = false)}>
				{t('myaaw.plugins.skill_dialog_close')}
			</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
