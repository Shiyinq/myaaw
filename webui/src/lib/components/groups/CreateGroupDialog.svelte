<script lang="ts">
	import { untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { Check, Loader2, Users } from 'lucide-svelte';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import Bloub from '$lib/components/common/Bloub.svelte';
	import { colorOf } from '$lib/bloub';
	import { agentSelection, groupsStore, toastStore } from '$lib/stores';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { getErrorMessage } from '$lib/utils/api';
	import type { DiscussionMode } from '$lib/types';

	const { t } = useTranslation();

	let name = $state('');
	let description = $state('');
	let selectedMembers = $state<string[]>([]);
	let discussionMode = $state<DiscussionMode>('autonomous');
	let maxTurns = $state(8);
	let submitting = $state(false);
	let formError = $state('');

	const agents = $derived(agentSelection.agents);
	const canSubmit = $derived(
		name.trim().length > 0 &&
			selectedMembers.length > 0 &&
			maxTurns >= 1 &&
			maxTurns <= 50 &&
			!submitting
	);

	const modeOptions: { id: DiscussionMode; labelKey: string; descKey: string }[] = [
		{
			id: 'autonomous',
			labelKey: 'myaaw.groups.mode_autonomous',
			descKey: 'myaaw.groups.mode_autonomous_desc'
		},
		{
			id: 'all',
			labelKey: 'myaaw.groups.mode_all',
			descKey: 'myaaw.groups.mode_all_desc'
		},
		{
			id: 'mention',
			labelKey: 'myaaw.groups.mode_mention',
			descKey: 'myaaw.groups.mode_mention_desc'
		}
	];

	// Reset form cleanly whenever the dialog opens.
	$effect(() => {
		if (!groupsStore.createDialogOpen) return;
		untrack(() => {
			name = '';
			description = '';
			discussionMode = 'autonomous';
			maxTurns = 8;
			formError = '';
			submitting = false;
			const running = agentSelection.agents.filter((a) => a.running).map((a) => a.name);
			selectedMembers =
				running.length > 0
					? running.slice(0, 4)
					: agentSelection.agents.slice(0, 2).map((a) => a.name);
		});
	});

	function toggleMember(agentName: string) {
		if (selectedMembers.includes(agentName)) {
			selectedMembers = selectedMembers.filter((m) => m !== agentName);
		} else {
			selectedMembers = [...selectedMembers, agentName];
		}
	}

	function selectAllMembers() {
		selectedMembers = agents.map((a) => a.name);
	}

	async function handleSubmit(event: SubmitEvent) {
		event.preventDefault();
		if (!canSubmit) return;
		submitting = true;
		formError = '';
		try {
			const created = await groupsStore.create({
				name: name.trim(),
				description: description.trim() || undefined,
				members: selectedMembers,
				discussion_mode: discussionMode,
				max_turns: Math.min(50, Math.max(1, Math.round(maxTurns)))
			});
			toastStore.success(t('myaaw.groups.created', { name: created.name }));
			groupsStore.closeCreateDialog();
			await goto(`/app/groups/${encodeURIComponent(created.id)}`);
		} catch (err) {
			formError = getErrorMessage(err);
		} finally {
			submitting = false;
		}
	}
</script>

<Dialog.Root
	open={groupsStore.createDialogOpen}
	onOpenChange={(open) => groupsStore.setCreateDialogOpen(open)}
>
	<Dialog.Content class="max-h-[90vh] overflow-y-auto sm:max-w-lg">
		<Dialog.Header>
			<Dialog.Title class="flex items-center gap-2">
				<Users class="h-4 w-4 text-primary" />
				{t('myaaw.groups.create_title')}
			</Dialog.Title>
			<Dialog.Description>{t('myaaw.groups.create_desc')}</Dialog.Description>
		</Dialog.Header>

		<form onsubmit={handleSubmit} class="space-y-4 pt-1">
			<div class="space-y-1.5">
				<Label for="group-name">{t('myaaw.groups.name_label')}</Label>
				<Input
					id="group-name"
					bind:value={name}
					maxlength={100}
					required
					placeholder={t('myaaw.groups.name_placeholder')}
				/>
			</div>

			<div class="space-y-1.5">
				<Label for="group-topic">{t('myaaw.groups.topic_label')}</Label>
				<textarea
					id="group-topic"
					bind:value={description}
					maxlength={2000}
					rows={2}
					placeholder={t('myaaw.groups.topic_placeholder')}
					class="w-full resize-none rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-xs outline-none placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50"
				></textarea>
			</div>

			<div class="space-y-2">
				<div class="flex items-center justify-between gap-2">
					<Label>
						{t('myaaw.groups.members_label')}
						<span class="ml-1 text-xs font-normal text-muted-foreground">
							({selectedMembers.length})
						</span>
					</Label>
					{#if agents.length > 1 && selectedMembers.length < agents.length}
						<button
							type="button"
							onclick={selectAllMembers}
							class="text-xs font-medium text-primary hover:underline"
						>
							{t('myaaw.groups.select_all')}
						</button>
					{/if}
				</div>

				{#if agents.length === 0}
					<p
						class="rounded-lg border border-dashed px-3 py-4 text-center text-xs text-muted-foreground"
					>
						{t('myaaw.groups.no_agents')}
					</p>
				{:else}
					<div class="grid max-h-44 grid-cols-1 gap-1.5 overflow-y-auto pr-1 sm:grid-cols-2">
						{#each agents as agent (agent.name)}
							{@const picked = selectedMembers.includes(agent.name)}
							<button
								type="button"
								onclick={() => toggleMember(agent.name)}
								aria-pressed={picked}
								class={[
									'flex items-center gap-2.5 rounded-xl border px-2.5 py-2 text-left transition-colors',
									picked
										? 'border-primary/60 bg-primary/8 text-foreground'
										: 'border-border bg-card hover:bg-muted/60'
								]}
							>
								<span class="relative shrink-0">
									<Bloub mood="attentif" size={26} tight color={colorOf(agent.agent_color)} />
									<span
										aria-hidden="true"
										class={[
											'absolute -right-0.5 -bottom-0.5 h-2 w-2 rounded-full ring-2 ring-card',
											agent.running ? 'bg-emerald-500' : 'bg-muted-foreground/40'
										]}
									></span>
								</span>
								<span class="min-w-0 flex-1">
									<span class="block truncate text-xs font-medium">{agent.name}</span>
									<span class="block text-[10px] text-muted-foreground">
										{agent.running ? t('myaaw.groups.online') : t('myaaw.bots.stopped')}
									</span>
								</span>
								<span
									class={[
										'grid h-4 w-4 shrink-0 place-items-center rounded-full border transition-colors',
										picked
											? 'border-primary bg-primary text-primary-foreground'
											: 'border-muted-foreground/40'
									]}
								>
									{#if picked}
										<Check class="h-2.5 w-2.5" />
									{/if}
								</span>
							</button>
						{/each}
					</div>
				{/if}
			</div>

			<div class="space-y-2">
				<Label>{t('myaaw.groups.mode_label')}</Label>
				<div class="grid grid-cols-1 gap-1.5 sm:grid-cols-3">
					{#each modeOptions as opt (opt.id)}
						{@const active = discussionMode === opt.id}
						<button
							type="button"
							onclick={() => (discussionMode = opt.id)}
							aria-pressed={active}
							class={[
								'flex flex-col rounded-xl border p-2.5 text-left transition-colors',
								active
									? 'border-primary bg-primary/8 text-foreground'
									: 'border-border hover:bg-muted/60'
							]}
						>
							<span class="text-xs font-semibold">{t(opt.labelKey)}</span>
							<span class="mt-0.5 text-[11px] leading-snug text-muted-foreground">
								{t(opt.descKey)}
							</span>
						</button>
					{/each}
				</div>
			</div>

			<div class="flex items-center justify-between gap-4 rounded-xl border px-3 py-2.5">
				<div class="min-w-0">
					<Label for="group-max-turns" class="text-xs font-medium">
						{t('myaaw.groups.max_turns_label')}
					</Label>
					<p class="text-[11px] text-muted-foreground">
						{t('myaaw.groups.max_turns_hint')}
					</p>
				</div>
				<Input
					id="group-max-turns"
					type="number"
					min={1}
					max={50}
					bind:value={maxTurns}
					class="w-20 text-center tabular-nums"
				/>
			</div>

			{#if formError}
				<p
					class="rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 text-xs text-destructive"
				>
					{formError}
				</p>
			{/if}

			<Dialog.Footer class="gap-2 pt-1">
				<Button
					type="button"
					variant="outline"
					onclick={() => groupsStore.closeCreateDialog()}
					disabled={submitting}
				>
					{t('common.cancel')}
				</Button>
				<Button type="submit" disabled={!canSubmit}>
					{#if submitting}
						<Loader2 class="mr-1.5 h-3.5 w-3.5 animate-spin" />
					{/if}
					{t('myaaw.groups.create_submit')}
				</Button>
			</Dialog.Footer>
		</form>
	</Dialog.Content>
</Dialog.Root>
