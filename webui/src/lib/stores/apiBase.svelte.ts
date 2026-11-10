import { DEFAULT_API_BASE, getApiBase, normalizeApiBase, setApiBase } from '$lib/constants';

let base = $state(getApiBase());

export const apiBase = {
	get value() {
		return base;
	},
	get isCustom() {
		return base !== DEFAULT_API_BASE;
	},
	set: (value: string) => {
		base = normalizeApiBase(value);
		setApiBase(base);
	},
	clear: () => {
		apiBase.set('');
	}
};
