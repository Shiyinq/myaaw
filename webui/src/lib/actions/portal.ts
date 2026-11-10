/**
 * Action to move the node to a target element (default: body)
 * Useful for modals and tooltips.
 */
export function portal(node: HTMLElement, target: HTMLElement | string = 'body') {
	let targetEl: HTMLElement | null;

	async function update(newTarget: HTMLElement | string) {
		targetEl = typeof newTarget === 'string' ? document.querySelector(newTarget) : newTarget;

		if (targetEl) {
			targetEl.appendChild(node);
		}
	}

	function destroy() {
		if (node.parentNode) {
			node.parentNode.removeChild(node);
		}
	}

	update(target);

	return {
		update,
		destroy
	};
}
