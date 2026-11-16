<script lang="ts">
	import { untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { MessageSquarePlus } from 'lucide-svelte';
	import ConversationList from '$lib/components/app/ConversationList.svelte';
	import SidebarAccount from '$lib/components/app/SidebarAccount.svelte';
	import SidebarAgent from '$lib/components/app/SidebarAgent.svelte';
	import SidebarBrand from '$lib/components/app/SidebarBrand.svelte';
	import { Button } from '$lib/components/ui/button';
	import { useTranslation } from '$lib/i18n/useTranslation';

	const { t } = useTranslation();

	interface Props {
		/** Icon-only rail: a new-chat button, since the list needs its labels. */
		compact?: boolean;
		/** The conversation the page is showing, highlighted in the list. */
		activeId?: string;
		/** Lets the shell focus the search box from a keyboard shortcut. */
		searchInput?: HTMLInputElement | null;
		/** Dismisses the phone drawer; desktop never shows the close button. */
		onClose: () => void;
	}

	let { compact = false, activeId = '', searchInput = $bindable(null), onClose }: Props = $props();

	// Swap the list for the compact new-chat button only after the sidebar width
	// animation finishes. Mounting the list while the rail is still narrow shows
	// clipped titles and looks like a glitch during expansion.
	let contentCompact = $state(untrack(() => compact));
	$effect(() => {
		const target = compact;
		if (target === contentCompact) return;
		const timer = setTimeout(() => (contentCompact = target), 200);
		return () => clearTimeout(timer);
	});

	function newChat() {
		void goto('/app/chat?new=1');
	}

	function openConversation(id: string) {
		void goto(`/app/chat/${id}`);
	}

	function newChatInFolder(folderId: string) {
		void goto(`/app/chat?new=1&folder=${encodeURIComponent(folderId)}`);
	}
</script>

<!--
	Classic mode is the chat-first shell: the sidebar keeps the brand and the
	agent picker, then becomes the conversation list, so there is no second list
	to find and no navigation to learn.
-->
<SidebarBrand {compact} {onClose} />
<SidebarAgent {compact} />

{#if contentCompact}
	<div class="flex h-14 shrink-0 items-center border-b px-3">
		<Button
			variant="ghost"
			size="icon-sm"
			class="flex"
			onclick={newChat}
			title={t('myaaw.chat.new')}
			aria-label={t('myaaw.chat.new')}
		>
			<MessageSquarePlus class="h-4 w-4" />
		</Button>
	</div>
	<div class="flex-1"></div>
{:else}
	<div class="flex min-h-0 flex-1 flex-col overflow-hidden">
		<ConversationList
			{activeId}
			onOpen={openConversation}
			onNew={newChat}
			onNewInFolder={newChatInFolder}
			onDeleted={(id) => {
				if (id === activeId) newChat();
			}}
			bind:searchInput
		/>
	</div>
{/if}

<SidebarAccount {compact} />
