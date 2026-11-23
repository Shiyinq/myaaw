<script lang="ts">
	import { CheckSquare, ClipboardList, MousePointerClick, Sparkles } from 'lucide-svelte';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import type { InteractionKind } from '$lib/types';

	interface Props {
		kind: InteractionKind;
	}

	let { kind }: Props = $props();

	const { t } = useTranslation();

	let Icon = $derived.by(() => {
		switch (kind) {
			case 'form':
				return ClipboardList;
			case 'checklist':
				return CheckSquare;
			case 'quick_reply':
				return Sparkles;
			default:
				return MousePointerClick;
		}
	});
</script>

<span
	class="inline-flex items-center gap-1.5 rounded-full border border-(--chat-bubble-ring) bg-secondary px-2.5 py-0.5 text-[11px] font-medium text-secondary-foreground"
>
	<Icon class="h-3 w-3 shrink-0" />
	{t(`myaaw.chat.interaction_${kind}`)}
</span>
