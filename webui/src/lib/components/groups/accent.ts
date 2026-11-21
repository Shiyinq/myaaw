import { colorOf, type BloubColor } from '$lib/bloub';

interface AccentPalette {
	dot: string;
	border: string;
	text: string;
	softBg: string;
	ring: string;
}

const ACCENTS: Record<BloubColor, AccentPalette> = {
	encre: {
		dot: 'bg-zinc-700 dark:bg-zinc-300',
		border: 'border-l-zinc-600 dark:border-l-zinc-400',
		text: 'text-zinc-800 dark:text-zinc-200',
		softBg: 'bg-zinc-500/15 dark:bg-zinc-400/20',
		ring: 'ring-zinc-500/25 dark:ring-zinc-400/30'
	},
	brun: {
		dot: 'bg-amber-800 dark:bg-amber-500',
		border: 'border-l-amber-700 dark:border-l-amber-500',
		text: 'text-amber-800 dark:text-amber-300',
		softBg: 'bg-amber-700/15 dark:bg-amber-500/20',
		ring: 'ring-amber-700/25 dark:ring-amber-500/30'
	},
	rouge: {
		dot: 'bg-red-500',
		border: 'border-l-red-500',
		text: 'text-red-700 dark:text-red-300',
		softBg: 'bg-red-500/15 dark:bg-red-500/20',
		ring: 'ring-red-500/25 dark:ring-red-400/30'
	},
	orange: {
		dot: 'bg-orange-500',
		border: 'border-l-orange-500',
		text: 'text-orange-700 dark:text-orange-300',
		softBg: 'bg-orange-500/15 dark:bg-orange-500/20',
		ring: 'ring-orange-500/25 dark:ring-orange-400/30'
	},
	ambre: {
		dot: 'bg-amber-500',
		border: 'border-l-amber-500',
		text: 'text-amber-700 dark:text-amber-300',
		softBg: 'bg-amber-500/15 dark:bg-amber-500/20',
		ring: 'ring-amber-500/25 dark:ring-amber-400/30'
	},
	vert: {
		dot: 'bg-emerald-500',
		border: 'border-l-emerald-500',
		text: 'text-emerald-700 dark:text-emerald-300',
		softBg: 'bg-emerald-500/15 dark:bg-emerald-500/20',
		ring: 'ring-emerald-500/25 dark:ring-emerald-400/30'
	},
	turquoise: {
		dot: 'bg-teal-500',
		border: 'border-l-teal-500',
		text: 'text-teal-700 dark:text-teal-300',
		softBg: 'bg-teal-500/15 dark:bg-teal-500/20',
		ring: 'ring-teal-500/25 dark:ring-teal-400/30'
	},
	bleu: {
		dot: 'bg-blue-500',
		border: 'border-l-blue-500',
		text: 'text-blue-700 dark:text-blue-300',
		softBg: 'bg-blue-500/15 dark:bg-blue-500/20',
		ring: 'ring-blue-500/25 dark:ring-blue-400/30'
	},
	violet: {
		dot: 'bg-violet-500',
		border: 'border-l-violet-500',
		text: 'text-violet-700 dark:text-violet-300',
		softBg: 'bg-violet-500/15 dark:bg-violet-500/20',
		ring: 'ring-violet-500/25 dark:ring-violet-400/30'
	},
	rose: {
		dot: 'bg-pink-500',
		border: 'border-l-pink-500',
		text: 'text-pink-700 dark:text-pink-300',
		softBg: 'bg-pink-500/15 dark:bg-pink-500/20',
		ring: 'ring-pink-500/25 dark:ring-pink-400/30'
	},
	gris: {
		dot: 'bg-slate-500',
		border: 'border-l-slate-500',
		text: 'text-slate-700 dark:text-slate-300',
		softBg: 'bg-slate-500/15 dark:bg-slate-400/20',
		ring: 'ring-slate-500/25 dark:ring-slate-400/30'
	},
	creme: {
		dot: 'bg-stone-400',
		border: 'border-l-stone-400',
		text: 'text-stone-700 dark:text-stone-200',
		softBg: 'bg-stone-500/15 dark:bg-stone-400/20',
		ring: 'ring-stone-500/25 dark:ring-stone-400/30'
	}
};

export function groupAccent(agentColor?: string | null): AccentPalette {
	return ACCENTS[colorOf(agentColor)];
}
