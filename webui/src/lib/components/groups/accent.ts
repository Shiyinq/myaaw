import { colorOf, type BloubColor } from '$lib/bloub';

interface AccentPalette {
	dot: string;
	border: string;
	text: string;
	softBg: string;
}

const ACCENTS: Record<BloubColor, AccentPalette> = {
	encre: {
		dot: 'bg-zinc-700 dark:bg-zinc-300',
		border: 'border-l-zinc-600 dark:border-l-zinc-400',
		text: 'text-zinc-700 dark:text-zinc-300',
		softBg: 'bg-zinc-500/10'
	},
	brun: {
		dot: 'bg-amber-800 dark:bg-amber-500',
		border: 'border-l-amber-700 dark:border-l-amber-500',
		text: 'text-amber-800 dark:text-amber-400',
		softBg: 'bg-amber-700/10'
	},
	rouge: {
		dot: 'bg-red-500',
		border: 'border-l-red-500',
		text: 'text-red-600 dark:text-red-400',
		softBg: 'bg-red-500/10'
	},
	orange: {
		dot: 'bg-orange-500',
		border: 'border-l-orange-500',
		text: 'text-orange-600 dark:text-orange-400',
		softBg: 'bg-orange-500/10'
	},
	ambre: {
		dot: 'bg-amber-500',
		border: 'border-l-amber-500',
		text: 'text-amber-600 dark:text-amber-400',
		softBg: 'bg-amber-500/10'
	},
	vert: {
		dot: 'bg-emerald-500',
		border: 'border-l-emerald-500',
		text: 'text-emerald-600 dark:text-emerald-400',
		softBg: 'bg-emerald-500/10'
	},
	turquoise: {
		dot: 'bg-teal-500',
		border: 'border-l-teal-500',
		text: 'text-teal-600 dark:text-teal-400',
		softBg: 'bg-teal-500/10'
	},
	bleu: {
		dot: 'bg-blue-500',
		border: 'border-l-blue-500',
		text: 'text-blue-600 dark:text-blue-400',
		softBg: 'bg-blue-500/10'
	},
	violet: {
		dot: 'bg-violet-500',
		border: 'border-l-violet-500',
		text: 'text-violet-600 dark:text-violet-400',
		softBg: 'bg-violet-500/10'
	},
	rose: {
		dot: 'bg-pink-500',
		border: 'border-l-pink-500',
		text: 'text-pink-600 dark:text-pink-400',
		softBg: 'bg-pink-500/10'
	},
	gris: {
		dot: 'bg-slate-500',
		border: 'border-l-slate-500',
		text: 'text-slate-600 dark:text-slate-400',
		softBg: 'bg-slate-500/10'
	},
	creme: {
		dot: 'bg-stone-400',
		border: 'border-l-stone-400',
		text: 'text-stone-600 dark:text-stone-300',
		softBg: 'bg-stone-400/15'
	}
};

export function groupAccent(agentColor?: string | null): AccentPalette {
	return ACCENTS[colorOf(agentColor)];
}
