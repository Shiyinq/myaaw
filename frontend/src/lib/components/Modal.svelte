<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import * as Dialog from '$lib/components/ui/dialog';

	interface Props {
		title?: string;
		message?: string;
		confirmText?: string;
		cancelText?: string;
		confirmVariant?: 'default' | 'destructive' | 'outline' | 'secondary' | 'ghost' | 'link';
		onconfirm?: () => void;
		oncancel?: () => void;
	}

	let {
		title = 'Confirm Action',
		message = 'Are you sure you want to proceed?',
		confirmText = 'Confirm',
		cancelText = 'Cancel',
		confirmVariant = 'default',
		onconfirm,
		oncancel
	}: Props = $props();

	function handleOpenChange(open: boolean) {
		if (!open && oncancel) {
			oncancel();
		}
	}
</script>

<Dialog.Root open={true} onOpenChange={handleOpenChange}>
	<Dialog.Portal>
		<Dialog.Overlay />
		<Dialog.Content class="sm:max-w-[425px]">
			<Dialog.Header>
				<Dialog.Title>{title}</Dialog.Title>
				<Dialog.Description>{message}</Dialog.Description>
			</Dialog.Header>
			<Dialog.Footer class="gap-2 sm:gap-0">
				<Button variant="ghost" onclick={oncancel}>{cancelText}</Button>
				<Button variant={confirmVariant} onclick={onconfirm}>{confirmText}</Button>
			</Dialog.Footer>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
