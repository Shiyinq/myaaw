<script lang="ts">
	import { untrack } from 'svelte';
	import { Check, FolderPlus, Search, Users } from 'lucide-svelte';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import Bloub from '$lib/components/common/Bloub.svelte';
	import RuntimeBadge from '$lib/components/common/RuntimeBadge.svelte';
	import { groupAccent } from '$lib/components/groups/accent';
	import { colorOf } from '$lib/bloub';
	import { agentSelection, groupsStore, toastStore } from '$lib/stores';
	import { botsSidebarStore } from '$lib/stores/botsSidebar.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import type { BotsSidebarItemKind, BotsSidebarItemRef } from '$lib/types';

	interface Props {
		open?: boolean;
		/** Existing custom section ID to edit, or null/empty to create a new section. */
		sectionId?: string | null;
	}

	let { open = $bindable(false), sectionId = null }: Props = $props();

	const { t } = useTranslation();

	let name = $state('');
	let selected = $state<BotsSidebarItemRef[]>([]);
	let query = $state('');

	const editingSection = $derived(sectionId ? botsSidebarStore.getSection(sectionId) : undefined);
	const isEditing = $derived(Boolean(editingSection));

	const allGroups = $derived(groupsStore.groups);
	const allAgents = $derived(agentSelection.agents);
	const totalItems = $derived(allGroups.length + allAgents.length);

	const needle = $derived(query.trim().toLowerCase());
	const filteredGroups = $derived.by(() => {
		if (!needle) return allGroups;
		return allGroups.filter((grp) => grp.name.toLowerCase().includes(needle));
	});
	const filteredAgents = $derived.by(() => {
		if (!needle) return allAgents;
		return allAgents.filter((bot) => bot.name.toLowerCase().includes(needle));
	});

	const canSubmit = $derived(name.trim().length > 0);

	$effect(() => {
		if (!open) return;
		untrack(() => {
			query = '';
			const existing = sectionId ? botsSidebarStore.getSection(sectionId) : undefined;
			if (existing) {
				name = existing.name;
				selected = existing.items.map((item) => ({ ...item }));
			} else {
				name = '';
				selected = [];
			}
		});
	});

	function isSelected(kind: BotsSidebarItemKind, id: string): boolean {
		return selected.some((item) => item.kind === kind && item.id === id);
	}

	function toggleItem(kind: BotsSidebarItemKind, id: string) {
		if (isSelected(kind, id)) {
			selected = selected.filter((item) => !(item.kind === kind && item.id === id));
		} else {
			selected = [...selected, { kind, id }];
		}
	}

	function otherSectionName(kind: BotsSidebarItemKind, id: string): string {
		const homeId = botsSidebarStore.homeSectionOf(kind, id);
		if (homeId === sectionId) return '';
		const sec = botsSidebarStore.getSection(homeId);
		return sec?.name ?? '';
	}

	function handleSubmit(event: SubmitEvent) {
		event.preventDefault();
		const trimmed = name.trim();
		if (!trimmed) return;
		if (editingSection) {
			if (botsSidebarStore.updateSection(editingSection.id, trimmed, selected)) {
				toastStore.success(t('myaaw.bots.section_updated', { name: trimmed }));
				open = false;
			}
		} else {
			const created = botsSidebarStore.createSection(trimmed, selected);
			if (created) {
				toastStore.success(t('myaaw.bots.section_created', { name: created.name }));
				open = false;
			}
		}
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Content
		class="inset-0 top-0 left-0 flex h-dvh max-h-dvh w-screen max-w-none translate-x-0 translate-y-0 flex-col gap-0 overflow-hidden rounded-none bg-background p-0 ring-0 sm:inset-auto sm:top-1/2 sm:left-1/2 sm:h-auto sm:max-h-[86vh] sm:w-full sm:max-w-lg sm:-translate-x-1/2 sm:-translate-y-1/2 sm:rounded-xl sm:bg-popover sm:ring-1"
	>
		<Dialog.Header
			class="shrink-0 border-b px-4 py-3.5 pr-12 text-left sm:border-b-0 sm:px-5 sm:pt-5 sm:pb-2"
		>
			<Dialog.Title class="flex items-center gap-2">
				<FolderPlus class="h-4 w-4 text-primary" />
				{isEditing ? t('myaaw.bots.section_edit_title') : t('myaaw.bots.section_create_title')}
			</Dialog.Title>
			<Dialog.Description>{t('myaaw.bots.section_create_desc')}</Dialog.Description>
		</Dialog.Header>

		<form
			onsubmit={handleSubmit}
			autocomplete="off"
			class="flex min-h-0 flex-1 flex-col overflow-hidden"
		>
			<div
				class="min-h-0 flex-1 space-y-4 overflow-y-auto overscroll-contain px-4 py-4 sm:px-5 sm:py-2"
			>
				<div class="space-y-1.5">
					<Label for="bots-section-name">{t('myaaw.bots.section_name_label')}</Label>
					<Input
						id="bots-section-name"
						bind:value={name}
						maxlength={64}
						required
						autofocus
						autocomplete="off"
						placeholder={t('myaaw.bots.section_name_placeholder')}
					/>
				</div>

				<div class="space-y-2">
					<div class="flex items-center justify-between gap-2">
						<Label>
							{t('myaaw.bots.section_items_label')}
							<span class="ml-1 text-xs font-normal text-muted-foreground">
								({selected.length})
							</span>
						</Label>
						{#if selected.length > 0}
							<button
								type="button"
								onclick={() => (selected = [])}
								class="text-xs font-medium text-muted-foreground hover:text-foreground hover:underline"
							>
								{t('myaaw.groups.clear_all')}
							</button>
						{/if}
					</div>
					<p class="text-[11px] text-muted-foreground">{t('myaaw.bots.section_items_hint')}</p>

					{#if totalItems > 6}
						<div class="relative">
							<Search
								class="pointer-events-none absolute top-1/2 left-2.5 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground"
							/>
							<Input
								bind:value={query}
								placeholder={t('common.search')}
								autocomplete="off"
								class="h-8 pl-8 text-xs"
							/>
						</div>
					{/if}

					{#if totalItems === 0}
						<p
							class="rounded-lg border border-dashed px-3 py-4 text-center text-xs text-muted-foreground"
						>
							{t('myaaw.groups.no_agents')}
						</p>
					{:else if filteredGroups.length === 0 && filteredAgents.length === 0}
						<p
							class="rounded-lg border border-dashed px-3 py-4 text-center text-xs text-muted-foreground"
						>
							{t('myaaw.bots.no_matches')}
						</p>
					{:else}
						<div class="space-y-3 sm:max-h-72 sm:overflow-y-auto sm:pr-1">
							{#if filteredGroups.length > 0}
								<div class="space-y-1.5">
									<p
										class="text-[10px] font-semibold tracking-widest text-muted-foreground uppercase"
									>
										{t('myaaw.groups.section_groups')}
									</p>
									<div class="grid grid-cols-1 gap-1.5 sm:grid-cols-2">
										{#each filteredGroups as grp (grp.id)}
											{@const picked = isSelected('group', grp.id)}
											{@const other = otherSectionName('group', grp.id)}
											<button
												type="button"
												onclick={() => toggleItem('group', grp.id)}
												aria-pressed={picked}
												class={[
													'flex items-center gap-2.5 rounded-xl border px-2.5 py-2 text-left transition-colors',
													picked
														? 'border-primary/60 bg-primary/8 text-foreground'
														: 'border-border bg-card hover:bg-muted/60'
												]}
											>
												<span
													class="grid h-7 w-7 shrink-0 place-items-center rounded-lg bg-muted/60 ring-1 ring-border/40"
												>
													{#if grp.members.length > 0}
														<span class="grid grid-cols-2 gap-0.5 p-1">
															{#each grp.members.slice(0, 4) as member (member.name)}
																<span
																	aria-hidden="true"
																	class={[
																		'h-2 w-2 rounded-full',
																		groupAccent(member.agent_color).dot
																	]}
																></span>
															{/each}
														</span>
													{:else}
														<Users class="h-3.5 w-3.5 text-muted-foreground" />
													{/if}
												</span>
												<span class="min-w-0 flex-1">
													<span class="block truncate text-xs font-medium">{grp.name}</span>
													<span class="block truncate text-[10px] text-muted-foreground">
														{#if other}
															{t('myaaw.bots.in_section_badge', { name: other })}
														{:else}
															{t('myaaw.groups.members_count', {
																count: grp.member_count || grp.members.length
															})}
														{/if}
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
								</div>
							{/if}

							{#if filteredAgents.length > 0}
								<div class="space-y-1.5">
									<p
										class="text-[10px] font-semibold tracking-widest text-muted-foreground uppercase"
									>
										{t('myaaw.groups.section_bots')}
									</p>
									<div class="grid grid-cols-1 gap-1.5 sm:grid-cols-2">
										{#each filteredAgents as agent (agent.name)}
											{@const picked = isSelected('agent', agent.name)}
											{@const other = otherSectionName('agent', agent.name)}
											<button
												type="button"
												onclick={() => toggleItem('agent', agent.name)}
												aria-pressed={picked}
												class={[
													'flex items-center gap-2.5 rounded-xl border px-2.5 py-2 text-left transition-colors',
													picked
														? 'border-primary/60 bg-primary/8 text-foreground'
														: 'border-border bg-card hover:bg-muted/60'
												]}
											>
												<span
													class="relative grid h-8 w-8 shrink-0 place-items-center rounded-lg bg-muted/60 ring-1 ring-border/40"
												>
													<Bloub
														mood="attentif"
														size={26}
														tight
														color={colorOf(agent.agent_color)}
													/>
													<RuntimeBadge
														runtimeType={agent.runtime_type}
														runtimePreset={agent.runtime_preset}
														class="-top-1.5 -left-1.5"
													/>
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
													<span class="block truncate text-[10px] text-muted-foreground">
														{#if other}
															{t('myaaw.bots.in_section_badge', { name: other })}
														{:else}
															{agent.running ? t('myaaw.groups.online') : t('myaaw.bots.stopped')}
														{/if}
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
								</div>
							{/if}
						</div>
					{/if}
				</div>
			</div>

			<Dialog.Footer
				class="mx-0 mb-0 shrink-0 flex-row justify-end gap-2 rounded-none border-t bg-muted/40 px-4 py-3 pb-[calc(0.75rem+env(safe-area-inset-bottom))] sm:rounded-b-xl sm:px-5 sm:py-3.5"
			>
				<Button type="button" variant="outline" onclick={() => (open = false)}>
					{t('common.cancel')}
				</Button>
				<Button type="submit" disabled={!canSubmit}>
					{isEditing ? t('myaaw.bots.section_save_submit') : t('myaaw.bots.section_create_submit')}
				</Button>
			</Dialog.Footer>
		</form>
	</Dialog.Content>
</Dialog.Root>
