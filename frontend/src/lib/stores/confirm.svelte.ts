import type { ComponentProps } from 'svelte';
import type Modal from '$lib/components/Modal.svelte';

export interface ConfirmOptions {
	title?: string;
	message: string;
	confirmText?: string;
	cancelText?: string;
	variant?: ComponentProps<typeof Modal>['confirmVariant'];
}

/**
 * Imperative confirmation dialog. `askConfirm` returns a promise that resolves
 * to true when the user confirms, so call sites can await it instead of
 * blocking on window.confirm.
 */
class ConfirmState {
	current = $state<ConfirmOptions | null>(null);
	#resolve: ((value: boolean) => void) | null = null;

	ask(options: ConfirmOptions): Promise<boolean> {
		// A second request replaces the first: the old one counts as cancelled.
		this.#resolve?.(false);
		this.current = options;
		return new Promise((resolve) => {
			this.#resolve = resolve;
		});
	}

	finish(value: boolean) {
		const resolve = this.#resolve;
		this.#resolve = null;
		this.current = null;
		resolve?.(value);
	}
}

export const confirmState = new ConfirmState();

export function askConfirm(options: ConfirmOptions): Promise<boolean> {
	return confirmState.ask(options);
}
