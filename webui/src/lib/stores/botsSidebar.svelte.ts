import { browser } from '$app/environment';
import { queueSettingsPatch } from '$lib/stores/settingsSync.svelte';
import type {
	BotsSidebarConfig,
	BotsSidebarItemKind,
	BotsSidebarItemRef,
	BotsSidebarSection
} from '$lib/types';

export const BOTS_SECTION_GROUPS = 'groups';
export const BOTS_SECTION_AGENTS = 'agents';
export const BOTS_SECTION_UNREAD = 'unread';

const MAX_SECTIONS = 30;
const MAX_NAME_LEN = 64;
const MAX_ID_LEN = 64;

const CONFIG_STORAGE_KEY = 'myaaw_bots_sidebar';
const COLLAPSED_STORAGE_KEY = 'myaaw_bots_sidebar_collapsed';

export function defaultBotsSidebarConfig(): BotsSidebarConfig {
	return {
		section_order: [BOTS_SECTION_GROUPS, BOTS_SECTION_AGENTS],
		sections: []
	};
}

export function normalizeBotsSidebarConfig(raw: unknown): BotsSidebarConfig {
	if (!raw || typeof raw !== 'object') {
		return defaultBotsSidebarConfig();
	}
	const src = raw as Partial<BotsSidebarConfig>;
	const rawSections = Array.isArray(src.sections) ? src.sections : [];
	const rawOrder = Array.isArray(src.section_order) ? src.section_order : [];

	const sections: BotsSidebarSection[] = [];
	const seenSec = new Set<string>();
	const seenItems = new Set<string>();

	for (const entry of rawSections) {
		if (sections.length >= MAX_SECTIONS) break;
		if (!entry || typeof entry !== 'object') continue;
		const id = typeof entry.id === 'string' ? entry.id.trim() : '';
		let name = typeof entry.name === 'string' ? entry.name.trim() : '';
		if (!id || !name || id.length > MAX_ID_LEN) continue;
		if (id === BOTS_SECTION_GROUPS || id === BOTS_SECTION_AGENTS || id === BOTS_SECTION_UNREAD) {
			continue;
		}
		if (seenSec.has(id)) continue;
		if (name.length > MAX_NAME_LEN) {
			name = name.slice(0, MAX_NAME_LEN);
		}
		const items: BotsSidebarItemRef[] = [];
		const rawItems = Array.isArray(entry.items) ? entry.items : [];
		for (const item of rawItems) {
			if (!item || typeof item !== 'object') continue;
			const kind = typeof item.kind === 'string' ? item.kind.trim().toLowerCase() : '';
			const itemId = typeof item.id === 'string' ? item.id.trim() : '';
			if ((kind !== 'agent' && kind !== 'group') || !itemId) continue;
			const key = `${kind}\0${itemId}`;
			if (seenItems.has(key)) continue;
			seenItems.add(key);
			items.push({ kind, id: itemId });
		}
		seenSec.add(id);
		sections.push({ id, name, items });
	}

	const order: string[] = [];
	const seenOrder = new Set<string>();
	for (const rawId of rawOrder) {
		const id = typeof rawId === 'string' ? rawId.trim() : '';
		if (!id || seenOrder.has(id)) continue;
		if (id !== BOTS_SECTION_GROUPS && id !== BOTS_SECTION_AGENTS && !seenSec.has(id)) continue;
		seenOrder.add(id);
		order.push(id);
	}
	for (const builtin of [BOTS_SECTION_GROUPS, BOTS_SECTION_AGENTS]) {
		if (!seenOrder.has(builtin)) {
			seenOrder.add(builtin);
			order.push(builtin);
		}
	}
	for (const sec of sections) {
		if (!seenOrder.has(sec.id)) {
			seenOrder.add(sec.id);
			order.push(sec.id);
		}
	}

	return { section_order: order, sections };
}

function loadCachedConfig(): BotsSidebarConfig {
	if (!browser) return defaultBotsSidebarConfig();
	try {
		const raw = localStorage.getItem(CONFIG_STORAGE_KEY);
		if (!raw) return defaultBotsSidebarConfig();
		return normalizeBotsSidebarConfig(JSON.parse(raw));
	} catch {
		return defaultBotsSidebarConfig();
	}
}

function saveCachedConfig(cfg: BotsSidebarConfig): void {
	if (!browser) return;
	try {
		localStorage.setItem(CONFIG_STORAGE_KEY, JSON.stringify(cfg));
	} catch {
		// Ignore quota errors in private browsing.
	}
}

function loadCollapsed(): Record<string, boolean> {
	if (!browser) return {};
	try {
		const raw = localStorage.getItem(COLLAPSED_STORAGE_KEY);
		if (!raw) return {};
		const parsed: unknown = JSON.parse(raw);
		if (!parsed || typeof parsed !== 'object') return {};
		const out: Record<string, boolean> = {};
		for (const [key, val] of Object.entries(parsed as Record<string, unknown>)) {
			if (typeof val === 'boolean') out[key] = val;
		}
		return out;
	} catch {
		return {};
	}
}

function saveCollapsed(state: Record<string, boolean>): void {
	if (!browser) return;
	try {
		localStorage.setItem(COLLAPSED_STORAGE_KEY, JSON.stringify(state));
	} catch {
		// Ignore quota errors in private browsing.
	}
}

function makeSectionId(): string {
	const rand = Math.random().toString(36).slice(2, 7);
	return `sec_${Date.now().toString(36)}_${rand}`;
}

function sameRef(a: BotsSidebarItemRef, b: BotsSidebarItemRef): boolean {
	return a.kind === b.kind && a.id === b.id;
}

let config = $state<BotsSidebarConfig>(loadCachedConfig());
let collapsed = $state<Record<string, boolean>>(loadCollapsed());

function commit(nextRaw: BotsSidebarConfig): BotsSidebarConfig {
	const normalized = normalizeBotsSidebarConfig(nextRaw);
	config = normalized;
	saveCachedConfig(normalized);
	queueSettingsPatch({ bots_sidebar: normalized });
	return normalized;
}

export const botsSidebarStore = {
	get config(): BotsSidebarConfig {
		return config;
	},

	get sectionOrder(): string[] {
		return config.section_order;
	},

	get sections(): BotsSidebarSection[] {
		return config.sections;
	},

	/** Hydrates synced layout from controlplane.db without echoing PATCH /settings. */
	hydrate(remote?: BotsSidebarConfig): void {
		const normalized = normalizeBotsSidebarConfig(remote);
		config = normalized;
		saveCachedConfig(normalized);
	},

	getSection(id: string): BotsSidebarSection | undefined {
		return config.sections.find((sec) => sec.id === id);
	},

	/** Returns the custom section ID containing this item, or the built-in section ID. */
	homeSectionOf(kind: BotsSidebarItemKind, id: string): string {
		for (const sec of config.sections) {
			if (sec.items.some((item) => item.kind === kind && item.id === id)) {
				return sec.id;
			}
		}
		return kind === 'group' ? BOTS_SECTION_GROUPS : BOTS_SECTION_AGENTS;
	},

	/** Enforces typed rules: built-in Groups accepts only groups, Agents accepts only agents. */
	canAcceptItem(targetSectionId: string, kind: BotsSidebarItemKind): boolean {
		if (targetSectionId === BOTS_SECTION_UNREAD) return false;
		if (targetSectionId === BOTS_SECTION_GROUPS) return kind === 'group';
		if (targetSectionId === BOTS_SECTION_AGENTS) return kind === 'agent';
		return config.sections.some((sec) => sec.id === targetSectionId);
	},

	/** Creates a new custom mixed section and places it at the top of section_order. */
	createSection(name: string, items: BotsSidebarItemRef[] = []): BotsSidebarSection | null {
		const trimmed = name.trim();
		if (!trimmed || config.sections.length >= MAX_SECTIONS) return null;
		const id = makeSectionId();
		const targetSet = new Set(items.map((item) => `${item.kind}\0${item.id}`));

		// Remove selected items from any previous custom section so 1-item-1-section holds.
		const updatedExisting = config.sections.map((sec) => ({
			...sec,
			items: sec.items.filter((item) => !targetSet.has(`${item.kind}\0${item.id}`))
		}));

		const created: BotsSidebarSection = {
			id,
			name: trimmed.slice(0, MAX_NAME_LEN),
			items: items.map((item) => ({ kind: item.kind, id: item.id.trim() }))
		};

		const next = commit({
			section_order: [id, ...config.section_order],
			sections: [created, ...updatedExisting]
		});
		return next.sections.find((sec) => sec.id === id) ?? null;
	},

	/** Updates a custom section's name and item membership (moving selected items from other sections). */
	updateSection(id: string, name: string, items: BotsSidebarItemRef[]): boolean {
		const trimmed = name.trim();
		if (!trimmed) return false;
		if (!config.sections.some((sec) => sec.id === id)) return false;

		const targetSet = new Set(items.map((item) => `${item.kind}\0${item.id}`));
		const nextSections = config.sections.map((sec) => {
			if (sec.id === id) {
				return {
					...sec,
					name: trimmed.slice(0, MAX_NAME_LEN),
					items: items.map((item) => ({ kind: item.kind, id: item.id.trim() }))
				};
			}
			return {
				...sec,
				items: sec.items.filter((item) => !targetSet.has(`${item.kind}\0${item.id}`))
			};
		});

		commit({
			section_order: config.section_order,
			sections: nextSections
		});
		return true;
	},

	renameSection(id: string, name: string): boolean {
		const sec = config.sections.find((entry) => entry.id === id);
		if (!sec) return false;
		return this.updateSection(id, name, sec.items);
	},

	/** Deletes a custom section; items inside automatically fall back to Agents / Groups. */
	deleteSection(id: string): boolean {
		if (!config.sections.some((sec) => sec.id === id)) return false;
		commit({
			section_order: config.section_order.filter((entry) => entry !== id),
			sections: config.sections.filter((sec) => sec.id !== id)
		});
		if (id in collapsed) {
			const nextCollapsed = { ...collapsed };
			delete nextCollapsed[id];
			collapsed = nextCollapsed;
			saveCollapsed(nextCollapsed);
		}
		return true;
	},

	/** Moves one agent or group into targetSectionId while respecting typed built-in rules. */
	moveItem(ref: BotsSidebarItemRef, targetSectionId: string): boolean {
		if (!this.canAcceptItem(targetSectionId, ref.kind)) return false;
		const currentHome = this.homeSectionOf(ref.kind, ref.id);
		if (currentHome === targetSectionId) return false;

		const nextSections = config.sections.map((sec) => {
			const without = sec.items.filter((item) => !sameRef(item, ref));
			if (sec.id === targetSectionId) {
				return { ...sec, items: [...without, { kind: ref.kind, id: ref.id }] };
			}
			return { ...sec, items: without };
		});

		commit({
			section_order: config.section_order,
			sections: nextSections
		});
		return true;
	},

	reorderSections(nextOrder: string[]): void {
		commit({
			section_order: nextOrder,
			sections: config.sections
		});
	},

	moveSectionRelative(draggedId: string, targetId: string, position: 'before' | 'after'): boolean {
		if (!draggedId || !targetId || draggedId === targetId) return false;
		const current = [...config.section_order];
		const fromIdx = current.indexOf(draggedId);
		const targetIdx = current.indexOf(targetId);
		if (fromIdx < 0 || targetIdx < 0) return false;

		current.splice(fromIdx, 1);
		const nextTargetIdx = current.indexOf(targetId);
		const insertIdx = position === 'before' ? nextTargetIdx : nextTargetIdx + 1;
		current.splice(insertIdx, 0, draggedId);
		if (current.every((id, idx) => id === config.section_order[idx])) {
			return false;
		}
		this.reorderSections(current);
		return true;
	},

	moveSectionBy(id: string, delta: -1 | 1): boolean {
		const current = [...config.section_order];
		const idx = current.indexOf(id);
		if (idx < 0) return false;
		const nextIdx = idx + delta;
		if (nextIdx < 0 || nextIdx >= current.length) return false;
		const [item] = current.splice(idx, 1);
		current.splice(nextIdx, 0, item);
		this.reorderSections(current);
		return true;
	},

	isCollapsed(sectionId: string): boolean {
		return collapsed[sectionId] === true;
	},

	toggleCollapsed(sectionId: string): void {
		const next = { ...collapsed, [sectionId]: !collapsed[sectionId] };
		collapsed = next;
		saveCollapsed(next);
	},

	setCollapsed(sectionId: string, value: boolean): void {
		if ((collapsed[sectionId] === true) === value) return;
		const next = { ...collapsed, [sectionId]: value };
		collapsed = next;
		saveCollapsed(next);
	}
};
