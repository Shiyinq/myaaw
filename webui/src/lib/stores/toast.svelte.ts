import type { Toast } from '$lib/types';
import { toast as sonnerToast } from 'svelte-sonner';

export const toastStore = {
	get items() {
		return [];
	},
	add: (message: string, type: Toast['type'] = 'info', duration = 3000) => {
		switch (type) {
			case 'success':
				sonnerToast.success(message, { duration });
				break;
			case 'error':
				sonnerToast.error(message, { duration });
				break;
			case 'warning':
				sonnerToast.warning(message, { duration });
				break;
			case 'info':
			default:
				sonnerToast.info(message, { duration });
				break;
		}
	},
	remove: (id: string) => {
		// sonner handles removal internally, or we can dismiss by id if we tracked it
		sonnerToast.dismiss(id);
	},
	success: (msg: string) => sonnerToast.success(msg),
	error: (msg: string) => sonnerToast.error(msg),
	info: (msg: string) => sonnerToast.info(msg),
	warning: (msg: string) => sonnerToast.warning(msg)
};

export const addToast = toastStore.add;
