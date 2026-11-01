<script lang="ts">
	import * as Dialog from '$lib/components/ui/dialog';
	import { shortcutHelp } from '$lib/stores/shortcutHelp.svelte';
	import { shortcutGroups } from '$lib/shortcuts';
	import { formatShortcut, isMac } from '$lib/utils/keys';
	import { useTranslation } from '$lib/i18n/useTranslation';

	const { t } = useTranslation();
</script>

<Dialog.Root open={shortcutHelp.open} onOpenChange={(value) => shortcutHelp.set(value)}>
	<Dialog.Portal>
		<Dialog.Overlay />
		<Dialog.Content class="sm:max-w-lg">
			<Dialog.Header>
				<Dialog.Title>{t('myaaw.shortcuts.title')}</Dialog.Title>
				<Dialog.Description>{t('myaaw.shortcuts.description')}</Dialog.Description>
			</Dialog.Header>

			<div class="max-h-[60vh] space-y-5 overflow-y-auto pr-1">
				{#each shortcutGroups as group (group.titleKey)}
					<section class="space-y-1.5">
						<p class="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
							{t(group.titleKey)}
						</p>
						{#each group.entries as entry (entry.combo)}
							<div class="flex items-center justify-between gap-4 text-sm">
								<span class="min-w-0 flex-1">{t(entry.labelKey)}</span>
								<span class="flex shrink-0 items-center gap-1.5">
									<kbd
										class="rounded border bg-muted px-1.5 py-0.5 font-mono text-[11px] text-muted-foreground"
									>
										{formatShortcut(entry.combo)}
									</kbd>
									{#if formatShortcut(entry.combo, isMac ? 'other' : 'mac') !== formatShortcut(entry.combo)}
										<span class="text-[10px] text-muted-foreground">
											{formatShortcut(entry.combo, isMac ? 'other' : 'mac')}
										</span>
									{/if}
								</span>
							</div>
						{/each}
					</section>
				{/each}
			</div>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
