<script lang="ts">
	import Modal from '$lib/components/Modal.svelte';
	import { confirmState } from '$lib/stores/confirm.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';

	const { t } = useTranslation();
</script>

{#if confirmState.current}
	<Modal
		title={confirmState.current.title ?? t('common.confirm_title')}
		message={confirmState.current.message}
		confirmText={confirmState.current.confirmText ?? t('common.confirm')}
		cancelText={confirmState.current.cancelText ?? t('common.cancel')}
		confirmVariant={confirmState.current.variant ?? 'default'}
		onconfirm={() => confirmState.finish(true)}
		oncancel={() => confirmState.finish(false)}
	/>
{/if}
