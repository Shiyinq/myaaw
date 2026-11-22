<script lang="ts">
	import { untrack } from 'svelte';
	import { Bot, Loader2, Plus, Save } from 'lucide-svelte';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import FieldError from '$lib/components/common/FieldError.svelte';
	import Switch from '$lib/components/common/Switch.svelte';
	import { contactsStore } from '$lib/stores/contacts.svelte';
	import { toastStore } from '$lib/stores';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { getErrorMessage } from '$lib/utils/api';
	import { validate, type FieldErrors } from '$lib/utils/validation';
	import { contactSchema } from '$lib/schemas';
	import type { ContactView } from '$lib/types';

	interface Props {
		open: boolean;
		agent: string;
		editing: ContactView | null;
		onOpenChange: (open: boolean) => void;
	}

	let { open, agent, editing, onOpenChange }: Props = $props();
	const { t } = useTranslation();

	let saving = $state(false);
	let formError = $state('');

	let name = $state('');
	let url = $state('');
	let token = $state('');
	let description = $state('');
	let clearToken = $state(false);
	let errors = $state<FieldErrors>({});

	// Sync form fields cleanly whenever the dialog opens or switches target.
	$effect(() => {
		if (!open) return;
		const target = editing;
		untrack(() => {
			name = target?.name ?? '';
			url = target?.url ?? '';
			description = target?.description ?? '';
			token = '';
			clearToken = false;
			errors = {};
			formError = '';
			saving = false;
		});
	});

	async function handleSubmit(event: SubmitEvent) {
		event.preventDefault();
		if (!agent || saving) return;

		const result = validate(contactSchema, { name, url, token, description });
		if (!result.ok) {
			errors = result.errors;
			return;
		}

		errors = {};
		formError = '';
		saving = true;
		try {
			if (editing) {
				await contactsStore.update(agent, editing.name, {
					name: result.data.name,
					url: result.data.url,
					// An empty field means "keep the stored token".
					token: result.data.token || undefined,
					description: result.data.description ?? '',
					clear_token: clearToken
				});
				toastStore.success(t('myaaw.contacts.updated'));
			} else {
				await contactsStore.create(agent, {
					name: result.data.name,
					url: result.data.url,
					token: result.data.token || undefined,
					description: result.data.description || undefined
				});
				toastStore.success(t('myaaw.contacts.saved'));
			}
			onOpenChange(false);
		} catch (err) {
			formError = getErrorMessage(err);
		} finally {
			saving = false;
		}
	}
</script>

<Dialog.Root {open} {onOpenChange}>
	<Dialog.Content
		class="inset-0 top-0 left-0 flex h-dvh max-h-dvh w-screen max-w-none translate-x-0 translate-y-0 flex-col gap-0 overflow-hidden rounded-none bg-background p-0 ring-0 sm:inset-auto sm:top-1/2 sm:left-1/2 sm:h-auto sm:max-h-[90vh] sm:w-full sm:max-w-lg sm:-translate-x-1/2 sm:-translate-y-1/2 sm:rounded-xl sm:bg-popover sm:ring-1"
	>
		<Dialog.Header
			class="shrink-0 border-b px-4 py-3.5 pr-12 text-left sm:border-b-0 sm:px-5 sm:pt-5 sm:pb-2"
		>
			<Dialog.Title class="flex items-center gap-2">
				<Bot class="h-4 w-4 text-primary" />
				{editing ? t('myaaw.contacts.edit') : t('myaaw.contacts.create')}
			</Dialog.Title>
			<Dialog.Description>
				{editing ? t('myaaw.contacts.edit_desc') : t('myaaw.contacts.create_desc')}
			</Dialog.Description>
		</Dialog.Header>

		<form
			onsubmit={handleSubmit}
			autocomplete="off"
			class="flex min-h-0 flex-1 flex-col overflow-hidden"
		>
			<div
				class="min-h-0 flex-1 space-y-4 overflow-y-auto overscroll-contain px-4 py-4 sm:px-5 sm:py-2"
			>
				<div class="space-y-1.5">
					<Label for="contact-name">{t('myaaw.contacts.name')}</Label>
					<Input id="contact-name" bind:value={name} placeholder="research" />
					<FieldError code={errors.name} />
				</div>

				<div class="space-y-1.5">
					<Label for="contact-url">{t('myaaw.contacts.url')}</Label>
					<Input
						id="contact-url"
						bind:value={url}
						placeholder="https://host/a2a"
						class="font-mono text-xs"
					/>
					<FieldError code={errors.url} />
				</div>

				<div class="space-y-2 rounded-xl border p-3">
					<div class="space-y-1.5">
						<Label for="contact-token" class="text-xs">{t('myaaw.contacts.token')}</Label>
						<Input
							id="contact-token"
							type="password"
							bind:value={token}
							disabled={editing ? clearToken : false}
						/>
						<p class="text-[11px] text-muted-foreground">
							{editing ? t('myaaw.contacts.token_hint_keep') : t('myaaw.contacts.token_hint')}
						</p>
					</div>

					{#if editing?.token.configured}
						<div class="h-px bg-border/60"></div>
						<div class="flex items-center justify-between gap-3 pt-0.5">
							<p class="text-xs font-medium text-foreground">{t('myaaw.contacts.clear_token')}</p>
							<Switch
								checked={clearToken}
								label={t('myaaw.contacts.clear_token')}
								onchange={(next) => (clearToken = next)}
							/>
						</div>
					{/if}
				</div>

				<div class="space-y-1.5">
					<Label for="contact-description">{t('myaaw.contacts.description')}</Label>
					<textarea
						id="contact-description"
						bind:value={description}
						rows={2}
						class="w-full resize-none rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-xs outline-none placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50"
					></textarea>
					<FieldError code={errors.description} />
				</div>

				{#if formError}
					<p
						class="rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 text-xs text-destructive"
					>
						{formError}
					</p>
				{/if}
			</div>

			<Dialog.Footer
				class="mx-0 mb-0 shrink-0 flex-row justify-end gap-2 rounded-none border-t bg-muted/40 px-4 py-3.5 sm:rounded-b-xl sm:px-5 sm:py-3.5"
			>
				<Button
					type="button"
					variant="outline"
					onclick={() => onOpenChange(false)}
					disabled={saving}
				>
					{t('common.cancel')}
				</Button>
				<Button type="submit" disabled={saving}>
					{#if saving}
						<Loader2 class="mr-1.5 h-3.5 w-3.5 animate-spin" />
					{:else if editing}
						<Save class="mr-1.5 h-3.5 w-3.5" />
					{:else}
						<Plus class="mr-1.5 h-3.5 w-3.5" />
					{/if}
					{editing ? t('common.save') : t('myaaw.contacts.create')}
				</Button>
			</Dialog.Footer>
		</form>
	</Dialog.Content>
</Dialog.Root>
