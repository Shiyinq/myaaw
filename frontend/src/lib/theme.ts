import type { ComponentType } from 'svelte';
import type { Icon } from 'lucide-svelte';
import {
	Accessibility,
	BookOpen,
	CloudFog,
	Contrast,
	Film,
	Grid3x3,
	Layers,
	Monitor,
	Moon,
	MoonStar,
	PenLine,
	Sparkles,
	Sun,
	SunMedium,
	Terminal,
	Type
} from 'lucide-svelte';
import { resetMode, setMode, setTheme } from 'mode-watcher';

export type ThemeMode = 'light' | 'dark' | 'system';

/**
 * A named theme is a `data-theme` value layered on top of a fixed mode. The
 * matching `:root[data-theme='…']` block in `app.css` restyles tokens (and, for
 * a few, the page background or the typeface), so adding a theme is usually a
 * registry entry here plus one CSS block — no component markup changes.
 * Translucent themes (glass, mist) share the named `frost-*` surface classes,
 * and only `terminal` needs a variant, because it re-reads the transcript.
 */
export type ThemeId =
	| 'glass'
	| 'carbon'
	| 'terminal'
	| 'paper'
	| 'blueprint'
	| 'graphite'
	| 'slate'
	| 'typewriter'
	| 'kontras'
	| 'kontras-light'
	| 'noir'
	| 'redup'
	| 'mist';

/**
 * One selectable appearance, offered in Settings and the command palette. The
 * plain Light, Dark and System entries are modes with no custom theme; the rest
 * set `data-theme` and switch to the mode they are designed for.
 */
export interface AppearanceOption {
	id: string;
	labelKey: string;
	mode: ThemeMode;
	theme: '' | ThemeId;
	aliases: string[];
	icon: ComponentType<Icon>;
}

export const APPEARANCE: AppearanceOption[] = [
	{
		id: 'light',
		labelKey: 'myaaw.settings.theme_light',
		mode: 'light',
		theme: '',
		aliases: ['light', 'terang'],
		icon: Sun
	},
	{
		id: 'paper',
		labelKey: 'myaaw.settings.theme_paper',
		mode: 'light',
		theme: 'paper',
		aliases: ['paper', 'kertas', 'read', 'baca', 'sepia'],
		icon: BookOpen
	},
	{
		id: 'slate',
		labelKey: 'myaaw.settings.theme_slate',
		mode: 'light',
		theme: 'slate',
		aliases: ['slate', 'cool', 'dingin', 'grey', 'abu'],
		icon: Layers
	},
	{
		id: 'graphite',
		labelKey: 'myaaw.settings.theme_graphite',
		mode: 'light',
		theme: 'graphite',
		aliases: ['graphite', 'grafit', 'print', 'printout', 'cetak'],
		icon: PenLine
	},
	{
		id: 'typewriter',
		labelKey: 'myaaw.settings.theme_typewriter',
		mode: 'light',
		theme: 'typewriter',
		aliases: ['typewriter', 'mesin tik', 'mono', 'light terminal'],
		icon: Type
	},
	{
		id: 'kontras-light',
		labelKey: 'myaaw.settings.theme_kontras_light',
		mode: 'light',
		theme: 'kontras-light',
		aliases: ['contrast light', 'kontras terang', 'accessibility', 'aksesibilitas'],
		icon: SunMedium
	},
	{
		id: 'mist',
		labelKey: 'myaaw.settings.theme_mist',
		mode: 'light',
		theme: 'mist',
		aliases: ['mist', 'kabut', 'frosted light', 'glass light'],
		icon: CloudFog
	},
	{
		id: 'dark',
		labelKey: 'myaaw.settings.theme_dark',
		mode: 'dark',
		theme: '',
		aliases: ['dark', 'gelap'],
		icon: Moon
	},
	{
		id: 'carbon',
		labelKey: 'myaaw.settings.theme_carbon',
		mode: 'dark',
		theme: 'carbon',
		aliases: ['carbon', 'karbon', 'black', 'hitam', 'amoled', 'oled'],
		icon: Contrast
	},
	{
		id: 'noir',
		labelKey: 'myaaw.settings.theme_noir',
		mode: 'dark',
		theme: 'noir',
		aliases: ['noir', 'grain', 'film', 'texture', 'tekstur'],
		icon: Film
	},
	{
		id: 'redup',
		labelKey: 'myaaw.settings.theme_redup',
		mode: 'dark',
		theme: 'redup',
		aliases: ['dim', 'redup', 'night', 'malam', 'low contrast'],
		icon: MoonStar
	},
	{
		id: 'kontras',
		labelKey: 'myaaw.settings.theme_kontras',
		mode: 'dark',
		theme: 'kontras',
		aliases: ['contrast', 'kontras', 'accessibility', 'aksesibilitas', 'high contrast'],
		icon: Accessibility
	},
	{
		id: 'blueprint',
		labelKey: 'myaaw.settings.theme_blueprint',
		mode: 'dark',
		theme: 'blueprint',
		aliases: ['blueprint', 'cetak biru', 'grid', 'grid paper', 'blue'],
		icon: Grid3x3
	},
	{
		id: 'terminal',
		labelKey: 'myaaw.settings.theme_terminal',
		mode: 'dark',
		theme: 'terminal',
		aliases: ['terminal', 'mono', 'monospace', 'console'],
		icon: Terminal
	},
	{
		id: 'glass',
		labelKey: 'myaaw.settings.theme_glass',
		mode: 'dark',
		theme: 'glass',
		aliases: ['glass', 'kaca', 'frosted', 'blur'],
		icon: Sparkles
	},
	{
		id: 'system',
		labelKey: 'myaaw.settings.theme_system',
		mode: 'system',
		theme: '',
		aliases: ['system', 'sistem', 'auto'],
		icon: Monitor
	}
];

/** Whether an appearance option is the one currently in effect. */
export function isAppearanceActive(
	option: AppearanceOption,
	mode: ThemeMode,
	theme: string | undefined
): boolean {
	if (option.theme) return theme === option.theme;
	return theme === '' && mode === option.mode;
}

/**
 * Applies an appearance option. Plain modes clear `data-theme` first so a
 * surface that was translucent (glass) or restyled never lingers; named themes
 * set the theme and then the mode it is designed for.
 */
export function applyAppearance(option: AppearanceOption): void {
	setTheme(option.theme);
	if (option.mode === 'system') resetMode();
	else setMode(option.mode);
}

/** Whether the current custom theme paints frosted sheets (glass or mist). */
export function isFrosted(theme: string | undefined): boolean {
	return theme === 'glass' || theme === 'mist';
}
