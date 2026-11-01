<script lang="ts">
	import { goto } from '$app/navigation';
	import { tick } from 'svelte';
	import * as Dialog from '$lib/components/ui/dialog';
	import { commandPalette } from '$lib/stores/commandPalette.svelte';
	import { scoreMatch, type Command } from '$lib/commands';
	import { translationsOf } from '$lib/i18n';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { Search } from 'lucide-svelte';

	let { commands }: { commands: Command[] } = $props();

	const { t } = useTranslation();

	let query = $state('');
	let active = $state(0);
	let input = $state<HTMLInputElement | null>(null);

	const GROUP_ORDER: Command['group'][] = ['agent', 'navigate', 'action'];
	const GROUP_LABELS: Record<Command['group'], string> = {
		navigate: 'myaaw.command.group_navigate',
		agent: 'myaaw.command.group_agent',
		action: 'myaaw.command.group_action'
	};

	/** Everything the palette searches for one command, in every language. */
	function haystack(command: Command): string[] {
		const values: string[] = [command.name ?? ''];
		if (command.labelKey) values.push(...translationsOf(command.labelKey));
		if (command.descriptionKey) values.push(...translationsOf(command.descriptionKey));
		if (command.aliases) values.push(...command.aliases);
		// The "switch to" template without its placeholder still matches "switch".
		values.push(...translationsOf('myaaw.command.switch_to').map((v) => v.replace('{name}', '')));
		return values;
	}

	function labelOf(command: Command): string {
		if (command.labelKey) return t(command.labelKey, command.params);
		return command.name ?? command.id;
	}

	const groups = $derived.by(() => {
		const scored = commands
			.map((command) => ({ command, score: scoreMatch(haystack(command), query) }))
			.filter((entry) => entry.score > 0)
			.sort((a, b) => b.score - a.score);
		return GROUP_ORDER.map((group) => ({
			group,
			items: scored.filter((entry) => entry.command.group === group).map((entry) => entry.command)
		})).filter((section) => section.items.length > 0);
	});

	const flat = $derived(groups.flatMap((section) => section.items));

	$effect(() => {
		if (!commandPalette.open) return;
		query = '';
		active = 0;
		void tick().then(() => input?.focus());
	});

	// Keep the highlighted row inside the list as the query narrows.
	$effect(() => {
		const index = active;
		// Read the result count so a shorter list scrolls back into view too.
		const count = flat.length;
		if (count === 0) return;
		void tick().then(() => {
			document.getElementById(`command-option-${index}`)?.scrollIntoView({ block: 'nearest' });
		});
	});

	async function run(command: Command) {
		commandPalette.hide();
		await tick();
		if (command.run) {
			await command.run();
			return;
		}
		if (command.href) await goto(command.href);
	}

	function onKeydown(event: KeyboardEvent) {
		if (event.key === 'ArrowDown') {
			event.preventDefault();
			active = flat.length === 0 ? 0 : (active + 1) % flat.length;
			return;
		}
		if (event.key === 'ArrowUp') {
			event.preventDefault();
			active = flat.length === 0 ? 0 : (active - 1 + flat.length) % flat.length;
			return;
		}
		if (event.key === 'Enter') {
			event.preventDefault();
			const command = flat[active];
			if (command) void run(command);
			return;
		}
		if (event.key === 'Escape') {
			event.preventDefault();
			commandPalette.hide();
		}
	}
</script>

<Dialog.Root open={commandPalette.open} onOpenChange={(value) => commandPalette.set(value)}>
	<Dialog.Portal>
		<Dialog.Overlay />
		<Dialog.Content class="top-[12%] max-w-xl translate-y-0 gap-0 overflow-hidden p-0 sm:max-w-xl">
			<Dialog.Title class="sr-only">{t('myaaw.command.title')}</Dialog.Title>
			<Dialog.Description class="sr-only">{t('myaaw.command.placeholder')}</Dialog.Description>

			<div class="flex items-center gap-2 border-b px-3">
				<Search class="h-4 w-4 shrink-0 text-muted-foreground" />
				<input
					bind:this={input}
					bind:value={query}
					oninput={() => (active = 0)}
					onkeydown={onKeydown}
					role="combobox"
					aria-expanded="true"
					aria-controls="command-list"
					aria-autocomplete="list"
					aria-activedescendant={flat.length > 0 ? `command-option-${active}` : undefined}
					placeholder={t('myaaw.command.placeholder')}
					class="h-12 w-full min-w-0 bg-transparent text-sm outline-none placeholder:text-muted-foreground"
				/>
				<kbd
					class="shrink-0 rounded border bg-muted px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground"
				>
					ESC
				</kbd>
			</div>

			<div id="command-list" role="listbox" class="max-h-[55vh] overflow-y-auto p-1.5">
				{#each groups as section (section.group)}
					<div class="px-2 pt-2 pb-1">
						<p class="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
							{t(GROUP_LABELS[section.group])}
						</p>
					</div>
					{#each section.items as command (command.id)}
						{@const index = flat.indexOf(command)}
						{@const Icon = command.icon}
						<button
							type="button"
							id={`command-option-${index}`}
							role="option"
							aria-selected={index === active}
							class={[
								'flex w-full items-center gap-3 rounded-md border-l-2 px-2 py-2 text-left text-sm transition-colors',
								index === active
									? 'border-primary bg-accent text-accent-foreground'
									: 'border-transparent text-foreground hover:bg-muted/60',
								command.destructive ? 'text-destructive' : ''
							]}
							onmouseenter={() => (active = index)}
							onclick={() => void run(command)}
						>
							<Icon class="h-4 w-4 shrink-0" />
							<span class="min-w-0 flex-1 truncate">{labelOf(command)}</span>
							{#if command.current}
								<span class="shrink-0 rounded-full border px-1.5 py-0.5 text-[10px]">
									{t('myaaw.command.current')}
								</span>
							{/if}
						</button>
					{/each}
				{/each}

				{#if flat.length === 0}
					<p class="px-3 py-6 text-center text-sm text-muted-foreground">
						{t('myaaw.command.empty')}
					</p>
				{/if}
			</div>

			<div
				class="flex items-center justify-between border-t px-3 py-2 text-[10px] text-muted-foreground"
			>
				<span>
					{t('myaaw.command.hint_move')}
					<kbd class="rounded border px-1 font-mono">↑↓</kbd>
				</span>
				<span>
					{t('myaaw.command.hint_select')}
					<kbd class="rounded border px-1 font-mono">↵</kbd>
				</span>
			</div>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
