<script lang="ts">
	import { Bot } from 'lucide-svelte';
	import * as Dialog from '$lib/components/ui/dialog';
	import AgentSetupForm from '$lib/components/onboard/AgentSetupForm.svelte';
	import { createAgentDialog } from '$lib/stores/createAgentDialog.svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';

	const { t } = useTranslation();

	const isReconfigure = $derived(createAgentDialog.agentName !== '');
	const title = $derived(
		isReconfigure
			? t('myaaw.onboard.reconfigure_title', { name: createAgentDialog.agentName })
			: t('myaaw.groups.new_agent')
	);
	const description = $derived(
		isReconfigure ? t('myaaw.onboard.reconfigure_subtitle') : t('myaaw.home.new_agent_desc')
	);
</script>

<Dialog.Root open={createAgentDialog.open} onOpenChange={(next) => createAgentDialog.setOpen(next)}>
	<Dialog.Content
		class="inset-0 top-0 left-0 flex h-dvh max-h-dvh w-screen max-w-none translate-x-0 translate-y-0 flex-col gap-0 overflow-hidden rounded-none bg-background p-0 ring-0 sm:inset-auto sm:top-1/2 sm:left-1/2 sm:h-[88vh] sm:max-h-[88vh] sm:w-[92vw] sm:max-w-3xl sm:-translate-x-1/2 sm:-translate-y-1/2 sm:rounded-xl sm:bg-popover sm:ring-1"
	>
		<Dialog.Header class="shrink-0 border-b px-4 py-3.5 pr-12 text-left sm:px-5 sm:py-4 sm:pr-12">
			<Dialog.Title class="flex items-center gap-2">
				<Bot class="h-4 w-4 text-primary" />
				{title}
			</Dialog.Title>
			<Dialog.Description>{description}</Dialog.Description>
		</Dialog.Header>

		{#if createAgentDialog.open}
			<AgentSetupForm
				layout="dialog"
				agentName={createAgentDialog.agentName}
				initialRuntime={createAgentDialog.runtimePreset}
				onCancel={() => createAgentDialog.close()}
				onSuccess={(target, created) => createAgentDialog.notifySuccess(target, created)}
			/>
		{/if}
	</Dialog.Content>
</Dialog.Root>
