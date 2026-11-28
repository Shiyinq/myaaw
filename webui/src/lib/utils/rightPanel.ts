import { browser } from '$app/environment';

export type RightPanelKind = 'info' | 'attachments' | 'project';
export type StudioHistoryTab = 'history' | 'projects';
export type GroupInspectorTab = 'members' | 'attachments' | 'settings';

const PREFIX = 'myaaw_right_panel:';

export interface RightPanelState {
	/** Which panel is open. */
	t: RightPanelKind;
	/** Whether the sidebar itself is visible. */
	o: boolean;
	/** Studio History subtab (DM chat). */
	s?: StudioHistoryTab;
	/** Group Inspector subtab (group rooms). */
	g?: GroupInspectorTab;
}

/**
 * One key per URL entity: the conversation id for a DM chat, the group id for a
 * group room, or `draft` for a brand-new chat that has no id yet. Keeping every
 * sidebar pref behind this single namespace avoids one key per agent, one key per
 * tab, and a global open key drifting apart.
 */
export function rightPanelKey(id: string): string {
	return PREFIX + (id || 'draft');
}

export function readRightPanel(id: string): RightPanelState | null {
	if (!browser) return null;
	try {
		const raw = sessionStorage.getItem(rightPanelKey(id));
		if (!raw) return null;
		const parsed = JSON.parse(raw) as Partial<RightPanelState>;
		if (parsed.t !== 'info' && parsed.t !== 'attachments' && parsed.t !== 'project') return null;
		return {
			t: parsed.t,
			o: Boolean(parsed.o),
			s: parsed.s === 'projects' ? 'projects' : 'history',
			g: parsed.g === 'attachments' || parsed.g === 'settings' ? parsed.g : 'members'
		};
	} catch {
		return null;
	}
}

export function writeRightPanel(id: string, state: RightPanelState): void {
	if (!browser) return;
	try {
		sessionStorage.setItem(rightPanelKey(id), JSON.stringify(state));
	} catch {
		// Ignore storage quota errors; the in-memory panel still works.
	}
}
