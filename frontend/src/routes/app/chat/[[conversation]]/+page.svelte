<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { chat as chatApi } from '$lib/apis/chat';
	import { conversations as conversationsApi } from '$lib/apis/conversations';
	import { agentSelection, toastStore } from '$lib/stores';
	import { getErrorMessage } from '$lib/utils/api';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import PageHeader from '$lib/components/app/PageHeader.svelte';
	import Markdown from '$lib/components/common/Markdown.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { Card, CardContent } from '$lib/components/ui/card';
	import {
		AlertTriangle,
		ArrowDownToLine,
		Brain,
		MessageSquarePlus,
		Minimize2,
		RotateCcw,
		Send,
		ShieldAlert,
		Square,
		Wrench
	} from 'lucide-svelte';
	import type {
		ApprovalDecision,
		ApprovalPrompt,
		Attachment,
		ChatStreamEvent,
		TimelineItem,
		Usage
	} from '$lib/types';

	const { t } = useTranslation();

	interface Item extends TimelineItem {
		localId?: string;
		resolved?: string;
	}

	let agent = $derived(agentSelection.selected);
	let conversationId = $state('');
	let items = $state<Item[]>([]);
	let input = $state('');
	let streaming = $state(false);
	let loading = $state(false);
	let pendingApproval = $state<ApprovalPrompt | null>(null);
	let usage = $state<Usage | null>(null);
	let attachments = $state<Attachment[]>([]);
	let error = $state('');
	let scroller = $state<HTMLDivElement | null>(null);

	let controller: AbortController | null = null;
	let counter = 0;
	let loadKey = '';

	function scrollToBottom() {
		if (scroller) scroller.scrollTop = scroller.scrollHeight;
	}

	$effect(() => {
		void items.length;
		scrollToBottom();
	});

	async function loadConversation(currentAgent: string, id: string) {
		if (!id) {
			items = [];
			usage = null;
			attachments = [];
			return;
		}
		loading = true;
		error = '';
		try {
			const detail = await conversationsApi.get(currentAgent, id, { limit: 0 });
			items = detail.messages
				.filter((message) => message.role === 'user' || message.role === 'assistant')
				.map((message, index) => ({
					kind: message.role === 'user' ? 'user' : 'text',
					text: message.content,
					localId: `history-${index}`
				}));
			usage = null;
			attachments = [];
		} catch (err) {
			error = getErrorMessage(err);
		} finally {
			loading = false;
			scrollToBottom();
		}
	}

	$effect(() => {
		const currentAgent = agentSelection.selected;
		const currentConversation = page.params.conversation ?? '';
		const key = `${currentAgent}:${currentConversation}`;
		if (!currentAgent || key === loadKey) return;
		loadKey = key;
		conversationId = currentConversation;
		void loadConversation(currentAgent, currentConversation);
	});

	function push(item: Item) {
		items = [...items, { ...item, localId: item.localId ?? `item-${++counter}` }];
	}

	function mergeLast(kind: string, text: string) {
		const last = items[items.length - 1];
		if (last && last.kind === kind) {
			items = [...items.slice(0, -1), { ...last, text: (last.text ?? '') + text }];
		} else {
			push({ kind, text });
		}
	}

	function setToolResult(id: string, result: string, isError?: boolean) {
		items = items.map((item) =>
			item.kind === 'tool' && item.id === id ? { ...item, result, is_error: isError } : item
		);
	}

	async function send() {
		const text = input.trim();
		if (!text || !agent || streaming) return;

		push({ kind: 'user', text });
		input = '';
		streaming = true;
		error = '';
		pendingApproval = null;
		usage = null;
		attachments = [];
		controller = new AbortController();

		try {
			await chatApi.stream(
				agent,
				{ text, conversation_id: conversationId || undefined },
				(event) => {
					const data = event.data as Record<string, any>;
					switch (event.event as ChatStreamEvent) {
						case 'thought':
							mergeLast('thought', data?.text ?? '');
							break;
						case 'chunk':
							mergeLast('text', data?.text ?? '');
							break;
						case 'tool':
							push({
								kind: 'tool',
								id: data?.id,
								name: data?.name,
								arguments: data?.arguments
							});
							break;
						case 'tool_result':
							setToolResult(data?.id, data?.result ?? '', data?.is_error);
							break;
						case 'retry':
							push({ kind: 'retry', attempt: data?.attempt });
							break;
						case 'compaction':
							push({
								kind: 'compaction',
								tokens_before: data?.tokens_before,
								tokens_after: data?.tokens_after,
								dropped_messages: data?.dropped_messages
							});
							break;
						case 'approval_required': {
							const prompt: ApprovalPrompt = {
								approval_id: data?.approval_id,
								tool_name: data?.tool_name,
								action: data?.action,
								args_summary: data?.args_summary,
								subagent: data?.subagent
							};
							pendingApproval = prompt;
							push({
								kind: 'approval',
								id: prompt.approval_id,
								name: prompt.tool_name,
								arguments: prompt.args_summary,
								subagent: prompt.subagent
							});
							break;
						}
						case 'approval_resolved': {
							pendingApproval = null;
							items = items.map((item) =>
								item.kind === 'approval' && item.id === data?.approval_id
									? { ...item, resolved: data?.decision ?? data?.reason ?? 'resolved' }
									: item
							);
							break;
						}
						case 'done': {
							const nextConversation = data?.conversation_id;
							if (nextConversation && nextConversation !== conversationId) {
								conversationId = nextConversation;
								loadKey = `${agent}:${nextConversation}`;
								void goto(`/app/chat/${nextConversation}`, {
									replaceState: true,
									noScroll: true
								});
							}
							if (data?.text) {
								const hasText = [...items].reverse().some((item) => item.kind === 'text');
								if (!hasText) push({ kind: 'text', text: data.text });
							}
							usage = data?.usage ?? null;
							attachments = data?.attachments ?? [];
							break;
						}
						case 'error': {
							error = data?.error ?? 'Stream error';
							push({ kind: 'error', text: error });
							break;
						}
					}
				},
				controller.signal
			);
		} catch (err) {
			if ((err as Error)?.name !== 'AbortError') {
				error = getErrorMessage(err);
				toastStore.error(error);
			}
		} finally {
			streaming = false;
			controller = null;
		}
	}

	async function decide(decision: ApprovalDecision) {
		const prompt = pendingApproval;
		if (!prompt || !agent) return;
		pendingApproval = null;
		try {
			await chatApi.decide(agent, prompt.approval_id, decision);
			items = items.map((item) =>
				item.kind === 'approval' && item.id === prompt.approval_id
					? { ...item, resolved: decision }
					: item
			);
		} catch (err) {
			error = getErrorMessage(err);
			toastStore.error(error);
		}
	}

	function stop() {
		controller?.abort();
		controller = null;
		streaming = false;
	}

	function newChat() {
		controller?.abort();
		controller = null;
		streaming = false;
		conversationId = '';
		items = [];
		usage = null;
		attachments = [];
		error = '';
		pendingApproval = null;
		loadKey = `${agent}:`;
		void goto('/app/chat');
	}

	function onKeydown(event: KeyboardEvent) {
		if (event.key === 'Enter' && !event.shiftKey && !event.isComposing) {
			event.preventDefault();
			void send();
		}
	}

	function attachmentUrl(attachment: Attachment) {
		return `data:${attachment.mime_type || 'application/octet-stream'};base64,${attachment.data_base64}`;
	}

	function isImage(attachment: Attachment) {
		return (attachment.mime_type ?? '').startsWith('image/');
	}
</script>

<div class="flex h-[calc(100vh-8.5rem)] flex-col gap-3">
	<PageHeader title={t('myaaw.chat.title')} description={t('myaaw.chat.subtitle')}>
		<Button variant="outline" size="sm" onclick={newChat}>
			<MessageSquarePlus class="mr-2 h-4 w-4" />
			{t('myaaw.chat.new')}
		</Button>
	</PageHeader>

	{#if !agent}
		<Card>
			<CardContent class="py-10 text-center text-sm text-muted-foreground">
				{t('myaaw.agent.none')}
			</CardContent>
		</Card>
	{:else}
		<Card class="flex min-h-0 flex-1 flex-col overflow-hidden">
			<CardContent class="flex min-h-0 flex-1 flex-col gap-3 p-4">
				<div class="flex items-center justify-between gap-2">
					<div class="flex items-center gap-2 text-xs text-muted-foreground">
						<Badge variant="outline">{agent}</Badge>
						{#if conversationId}
							<span class="font-mono">{conversationId.slice(0, 12)}…</span>
						{/if}
					</div>
					{#if usage}
						<span class="text-[11px] text-muted-foreground">
							{t('myaaw.chat.tokens', { count: usage.total_tokens })}
						</span>
					{/if}
				</div>

				{#if error}
					<div
						class="rounded-md border border-destructive/30 bg-destructive/10 p-2 text-xs text-destructive"
					>
						{error}
					</div>
				{/if}

				<div bind:this={scroller} class="min-h-0 flex-1 space-y-3 overflow-y-auto pr-1">
					{#if loading}
						<p class="text-sm text-muted-foreground">{t('myaaw.chat.loading')}</p>
					{:else if items.length === 0}
						<p class="text-sm text-muted-foreground">{t('myaaw.chat.empty')}</p>
					{/if}

					{#each items as item, index (item.localId ?? `${item.kind}-${index}`)}
						{#if item.kind === 'user'}
							<div class="flex justify-end">
								<div
									class="max-w-[85%] whitespace-pre-wrap rounded-2xl rounded-br-sm bg-primary px-3 py-2 text-sm text-primary-foreground"
								>
									{item.text}
								</div>
							</div>
						{:else if item.kind === 'thought'}
							<div class="flex items-start gap-2 text-xs italic text-muted-foreground">
								<Brain class="mt-0.5 h-3.5 w-3.5 shrink-0" />
								<span class="whitespace-pre-wrap">{item.text}</span>
							</div>
						{:else if item.kind === 'text'}
							<div class="max-w-[90%] rounded-2xl rounded-bl-sm border bg-muted/30 px-3 py-2">
								<Markdown content={item.text ?? ''} />
							</div>
						{:else if item.kind === 'tool'}
							<div class="max-w-[90%] rounded-lg border bg-card p-2 text-xs">
								<div class="flex items-center gap-2 font-medium">
									<Wrench class="h-3.5 w-3.5" />
									<span class="font-mono">{item.name}</span>
								</div>
								{#if item.arguments}
									<pre class="mt-1 overflow-x-auto whitespace-pre-wrap break-all text-muted-foreground">{item.arguments}</pre>
								{/if}
								{#if item.result}
									<div class={item.is_error ? 'mt-1 text-destructive' : 'mt-1 text-muted-foreground'}>
										<div class="mb-0.5 flex items-center gap-1 font-medium">
											<ArrowDownToLine class="h-3 w-3" />
											{t('myaaw.chat.result')}
										</div>
										<pre class="overflow-x-auto whitespace-pre-wrap break-all">{item.result}</pre>
									</div>
								{/if}
							</div>
						{:else if item.kind === 'retry'}
							<div class="flex items-center gap-1 text-[11px] text-muted-foreground">
								<RotateCcw class="h-3 w-3" />
								{t('myaaw.chat.retry', { attempt: item.attempt ?? 2 })}
							</div>
						{:else if item.kind === 'compaction'}
							<div class="flex items-center gap-1 text-[11px] text-muted-foreground">
								<Minimize2 class="h-3 w-3" />
								{t('myaaw.chat.compaction', {
									before: item.tokens_before ?? 0,
									after: item.tokens_after ?? 0
								})}
							</div>
						{:else if item.kind === 'approval'}
							<div
								class="max-w-[90%] rounded-lg border border-amber-500/40 bg-amber-500/10 p-3 text-xs"
							>
								<div class="flex items-center gap-2 font-medium text-amber-700 dark:text-amber-300">
									<ShieldAlert class="h-4 w-4" />
									{t('myaaw.chat.waiting')}
								</div>
								<div class="mt-1 font-mono text-foreground">{item.name}</div>
								{#if item.arguments}
									<pre class="mt-1 overflow-x-auto whitespace-pre-wrap break-all text-muted-foreground">{item.arguments}</pre>
								{/if}
								{#if item.resolved}
									<Badge variant="outline" class="mt-2">{item.resolved}</Badge>
								{:else if pendingApproval && pendingApproval.approval_id === item.id}
									<div class="mt-2 flex flex-wrap gap-2">
										<Button size="xs" onclick={() => decide('allow_once')}>
											{t('myaaw.chat.allow_once')}
										</Button>
										<Button size="xs" variant="secondary" onclick={() => decide('allow_session')}>
											{t('myaaw.chat.allow_session')}
										</Button>
										<Button size="xs" variant="secondary" onclick={() => decide('allow_always')}>
											{t('myaaw.chat.allow_always')}
										</Button>
										<Button size="xs" variant="destructive" onclick={() => decide('deny')}>
											{t('myaaw.chat.deny')}
										</Button>
									</div>
								{/if}
							</div>
						{:else if item.kind === 'error'}
							<div class="flex items-start gap-2 text-xs text-destructive">
								<AlertTriangle class="mt-0.5 h-3.5 w-3.5 shrink-0" />
								<span class="whitespace-pre-wrap">{item.text}</span>
							</div>
						{/if}
					{/each}

					{#if attachments.length > 0}
						<div class="space-y-2">
							{#each attachments as attachment (attachment.file_name)}
								{#if isImage(attachment)}
									<img
										src={attachmentUrl(attachment)}
										alt={attachment.file_name}
										class="max-h-72 rounded-lg border"
									/>
								{:else}
									<a
										href={attachmentUrl(attachment)}
										download={attachment.file_name}
										class="inline-flex items-center gap-2 text-xs text-primary underline-offset-4 hover:underline"
									>
										<ArrowDownToLine class="h-3.5 w-3.5" />
										{attachment.file_name}
									</a>
								{/if}
							{/each}
						</div>
					{/if}
				</div>

				<div class="flex items-end gap-2 border-t pt-3">
					<textarea
						bind:value={input}
						onkeydown={onKeydown}
						rows="2"
						placeholder={t('myaaw.chat.placeholder')}
						class="min-h-10 w-full resize-none rounded-md border border-input bg-transparent px-3 py-2 text-sm outline-none transition-colors placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
					></textarea>
					{#if streaming}
						<Button variant="destructive" size="sm" onclick={stop}>
							<Square class="mr-2 h-4 w-4" />
							{t('myaaw.chat.stop')}
						</Button>
					{:else}
						<Button size="sm" onclick={send} disabled={!input.trim()}>
							<Send class="mr-2 h-4 w-4" />
							{t('myaaw.chat.send')}
						</Button>
					{/if}
				</div>
			</CardContent>
		</Card>
	{/if}
</div>
