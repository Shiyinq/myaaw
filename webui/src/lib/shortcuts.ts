/**
 * The single list of keyboard shortcuts. It feeds the help dialog, the tooltips
 * and the settings page, so a binding is only ever described in one place.
 */
export interface ShortcutEntry {
	combo: string;
	labelKey: string;
}

export interface ShortcutGroup {
	titleKey: string;
	entries: ShortcutEntry[];
}

export const shortcutGroups: ShortcutGroup[] = [
	{
		titleKey: 'myaaw.shortcuts.group_general',
		entries: [
			{ combo: 'mod+k', labelKey: 'myaaw.shortcuts.command' },
			{ combo: 'mod+backslash', labelKey: 'myaaw.shortcuts.sidebar' },
			{ combo: '?', labelKey: 'myaaw.shortcuts.help' }
		]
	},
	{
		titleKey: 'myaaw.shortcuts.group_chat',
		entries: [
			{ combo: 'mod+enter', labelKey: 'myaaw.shortcuts.send' },
			{ combo: '/', labelKey: 'myaaw.shortcuts.focus' },
			{ combo: 'mod+shift+f', labelKey: 'myaaw.shortcuts.attach' },
			{ combo: 'mod+shift+a', labelKey: 'myaaw.shortcuts.attachments' },
			{ combo: 'mod+shift+h', labelKey: 'myaaw.shortcuts.history' },
			{ combo: 'mod+shift+s', labelKey: 'myaaw.shortcuts.search' },
			{ combo: 'alt+arrowup', labelKey: 'myaaw.shortcuts.prev_turn' },
			{ combo: 'alt+arrowdown', labelKey: 'myaaw.shortcuts.next_turn' },
			{ combo: 'alt+n', labelKey: 'myaaw.shortcuts.new_chat' },
			{ combo: 'escape', labelKey: 'myaaw.shortcuts.interrupt' }
		]
	},
	{
		titleKey: 'myaaw.shortcuts.group_speech',
		entries: [{ combo: 'mod+shift+space', labelKey: 'myaaw.shortcuts.dictate' }]
	}
];
