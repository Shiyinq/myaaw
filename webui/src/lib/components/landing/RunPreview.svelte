<script lang="ts">
	import RunTrajectory from '$lib/components/common/RunTrajectory.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import {
		Card,
		CardContent,
		CardDescription,
		CardHeader,
		CardTitle
	} from '$lib/components/ui/card';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import type { Span } from '$lib/types';

	const { t } = useTranslation();
	const model = 'gemini-2.5-flash';
	const stats = { done: 6, total: 8, pending: 2, percent: 75 };

	// Synthetic recorder-shaped spans for the same weekly-report task as the chat demo.
	const spans: Span[] = $derived.by(() => {
		const calls = [
			{
				name: 'bash',
				args: { command: 'find notes data reports -maxdepth 2 -type f | sort', async: false }
			},
			{
				name: 'filesystem',
				args: {
					tool_name: 'read_multiple_files',
					path: JSON.stringify(['notes/week.md', 'data/tasks.csv'])
				}
			},
			{
				name: 'execute_python',
				args: {
					code: 'import csv, json, sys\nrows = list(csv.DictReader(sys.stdin))\ndone = sum(row["status"] == "done" for row in rows)\nprint(json.dumps({"done": done, "total": len(rows), "percent": round(done / len(rows) * 100)}))',
					input:
						'id,task,status\n1,landing,done\n2,navigation,done\n3,chat,done\n4,tools,done\n5,docs,done\n6,tests,done\n7,mobile_qa,pending\n8,release_review,pending\n',
					async: false
				}
			},
			{
				name: 'filesystem',
				args: {
					tool_name: 'write_file',
					path: 'reports/weekly.md',
					content: t('landing.stream.report_content', stats)
				}
			}
		];
		const result: Span[] = [
			{
				id: 'example_run',
				kind: 'run',
				name: 'Myaaw',
				started_at: 0,
				ended_at: 14000,
				duration_ms: 14000,
				status: 'completed'
			},
			{
				id: 'example_system',
				parent_id: 'example_run',
				kind: 'system',
				name: 'system',
				started_at: 0,
				duration_ms: 0,
				input_json: 'AGENTS.md · SOUL.md · TOOLS.md',
				status: 'completed'
			},
			{
				id: 'example_user',
				parent_id: 'example_run',
				kind: 'user',
				name: 'user',
				started_at: 0,
				duration_ms: 0,
				input_json: t('landing.stream.report_prompt'),
				status: 'completed'
			}
		];
		let start = 0;
		for (let index = 0; index < 5; index += 1) {
			const call = calls[index];
			const modelMs = index === 0 ? 1200 : 1400;
			const stepMs = modelMs + (call ? 1800 : 0);
			const stepId = `example_step_${index + 1}`;
			result.push({
				id: stepId,
				parent_id: 'example_run',
				kind: 'step',
				name: `Step ${index + 1}`,
				step_index: index + 1,
				started_at: start,
				ended_at: start + stepMs,
				duration_ms: stepMs,
				status: 'completed'
			});
			result.push({
				id: `example_llm_${index + 1}`,
				parent_id: stepId,
				kind: 'llm',
				name: model,
				step_index: index + 1,
				started_at: start,
				ended_at: start + modelMs,
				duration_ms: modelMs,
				input_tokens: 400 + index * 200,
				output_tokens: call ? 40 : 80,
				cost_usd: 0.0003,
				status: 'completed',
				output_json: JSON.stringify(
					call
						? {
								...(index === 0 ? { thought: t('landing.stream.thinking_detail') } : {}),
								tool_calls: [
									{ function: { name: call.name, arguments: JSON.stringify(call.args) } }
								]
							}
						: { content: t('landing.stream.report_answer', stats) }
				)
			});
			if (call)
				result.push({
					id: `example_tool_${index + 1}`,
					parent_id: stepId,
					kind: 'tool',
					name: call.name,
					step_index: index + 1,
					started_at: start + modelMs,
					ended_at: start + stepMs,
					duration_ms: 1800,
					input_json: JSON.stringify(call.args),
					status: 'completed'
				});
			start += stepMs;
		}
		return result;
	});
</script>

<Card class="run-preview min-w-0">
	<CardHeader class="shrink-0">
		<CardTitle class="flex flex-wrap items-center gap-2 text-sm">
			{t('myaaw.observability.trajectory')}
			<Badge variant="outline" class="text-[10px]">{t('landing.loop.legend')}</Badge>
		</CardTitle>
		<CardDescription class="text-xs wrap-anywhere">
			example_run · {model} · 14.00s · $0.0015
		</CardDescription>
	</CardHeader>
	<!-- Match the application's independently scrollable trajectory panel. -->

	<CardContent
		class="h-128 min-h-0 min-w-0 space-y-1 overflow-y-auto overscroll-contain"
		role="region"
		aria-label={t('myaaw.observability.trajectory')}
		tabindex={0}
	>
		<RunTrajectory {spans} />
	</CardContent>
</Card>
