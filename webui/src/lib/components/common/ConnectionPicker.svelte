<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { DEFAULT_API_BASE, normalizeApiBase } from '$lib/constants';
	import { connections } from '$lib/stores';
	import { Trash2 } from 'lucide-svelte';

	interface Props {
		value?: string;
		inputId?: string;
	}

	let { value = $bindable(''), inputId = 'server-url' }: Props = $props();
	const { t } = useTranslation();

	let selectedId = $state('__custom');
	let custom = $state('');

	const selected = $derived(connections.list.find((entry) => entry.id === selectedId));
	// Local is the fallback control plane, so it cannot be removed.
	const canRemove = $derived(selectedId !== '__custom' && selected?.url !== DEFAULT_API_BASE);

	$effect(() => {
		const normalized = normalizeApiBase(value);
		const match = connections.list.find((entry) => entry.url === normalized);
		if (match) {
			selectedId = match.id;
			custom = match.url;
			return;
		}
		if (normalized === DEFAULT_API_BASE && connections.list.length > 0) {
			// No local entry exists; prefer the first saved remote.
			const first = connections.list[0];
			selectedId = first.id;
			custom = first.url;
			value = first.url;
			return;
		}
		selectedId = '__custom';
		custom = value;
	});

	function choose(id: string) {
		selectedId = id;
		if (id === '__custom') {
			custom = value;
			return;
		}
		const connection = connections.list.find((entry) => entry.id === id);
		if (connection) {
			custom = connection.url;
			value = connection.url;
		}
	}

	function add() {
		const entry = connections.add('', custom);
		selectedId = entry.id;
		value = entry.url;
	}

	function remove() {
		const connection = connections.list.find((entry) => entry.id === selectedId);
		if (!connection || connection.url === DEFAULT_API_BASE) return;
		connections.remove(connection.id);
		const next = connections.list[0];
		selectedId = next?.id ?? '__custom';
		custom = next?.url ?? '';
		value = custom;
	}
</script>

<div class="grid gap-2">
	{#if connections.list.length > 0}
		<div class="flex gap-2">
			<select
				id={inputId}
				class="h-9 w-full rounded-md border border-input bg-transparent px-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
				value={selectedId}
				onchange={(event) => choose(event.currentTarget.value)}
			>
				{#each connections.list as connection (connection.id)}
					<option value={connection.id}>
						{connection.url === DEFAULT_API_BASE
							? `Local — ${t('myaaw.connection.local')}`
							: connection.url}
					</option>
				{/each}
				<option value="__custom">{t('myaaw.connection.custom')}</option>
			</select>
			{#if canRemove}
				<Button
					variant="ghost"
					size="icon"
					class="shrink-0"
					onclick={remove}
					title={t('myaaw.connection.remove')}
				>
					<Trash2 class="size-4" />
				</Button>
			{/if}
		</div>
	{/if}

	{#if selectedId === '__custom'}
		<Input
			id={`${inputId}-custom`}
			type="text"
			inputmode="url"
			placeholder={t('myaaw.connection.placeholder')}
			value={custom}
			oninput={(event) => {
				custom = event.currentTarget.value;
				value = custom;
			}}
		/>
		<Button variant="outline" size="sm" class="w-fit" onclick={add} disabled={custom.trim() === ''}>
			{t('myaaw.connection.add')}
		</Button>
	{/if}

	<p class="truncate text-xs text-muted-foreground">{t('myaaw.connection.hint')}</p>
</div>
