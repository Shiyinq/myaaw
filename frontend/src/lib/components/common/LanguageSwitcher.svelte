<script lang="ts">
	import { useTranslation } from '$lib/i18n/useTranslation';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu/index.js';
	import { Button } from '$lib/components/ui/button';
	import { Check, ChevronDown } from 'lucide-svelte';

	const { locale, changeLocale, availableLocales } = useTranslation();

	function selectLocale(code: any) {
		changeLocale(code);
	}
</script>

<DropdownMenu.Root>
	<DropdownMenu.Trigger>
		{#snippet child({ props })}
			<Button
				{...props}
				variant="outline"
				class="flex items-center gap-2 px-3 h-9 font-semibold border-border"
			>
				<span class="text-lg leading-none"
					>{availableLocales.find((l) => l.code === locale.value)?.flag}</span
				>
				<span class="text-sm uppercase tracking-wider">{locale.value}</span>
				<ChevronDown class="w-3 h-3 opacity-50" />
			</Button>
		{/snippet}
	</DropdownMenu.Trigger>
	<DropdownMenu.Content align="end" class="min-w-[180px] p-1">
		{#each availableLocales as loc}
			<DropdownMenu.Item
				onclick={() => selectLocale(loc.code)}
				class="flex items-center gap-3 py-2.5 px-3 cursor-pointer rounded-md transition-colors {locale.value ===
				loc.code
					? 'bg-accent text-accent-foreground'
					: 'hover:bg-muted'}"
			>
				<span class="text-xl leading-none">{loc.flag}</span>
				<div class="flex flex-col">
					<span class="text-sm font-medium">{loc.nativeName}</span>
				</div>
				{#if locale.value === loc.code}
					<Check class="w-4 h-4 ml-auto text-primary" />
				{/if}
			</DropdownMenu.Item>
		{/each}
	</DropdownMenu.Content>
</DropdownMenu.Root>
