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
	<Dialog.Content class="max-h-[85dvh] overflow-y-auto sm:max-w-lg">
		<Dialog.Header>
			<Dialog.Title>{t('myaaw.plugins.skill_dialog_title')}</Dialog.Title>
			<Dialog.Description>{t('myaaw.plugins.skill_dialog_body')}</Dialog.Description>
		</Dialog.Header>

		<div class="space-y-4">
			<div class="flex flex-col gap-2 rounded-lg border bg-muted/40 p-3">
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
					class="rounded-lg border bg-background p-3 font-mono text-xs leading-relaxed whitespace-pre-wrap"
				>
					{prompt}
				</p>
			</div>
		</div>

		<Dialog.Footer>
			<Button variant="outline" onclick={() => (open = false)}>
				{t('myaaw.plugins.skill_dialog_close')}
			</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
