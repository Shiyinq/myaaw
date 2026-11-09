<script lang="ts">
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { useTranslation } from '$lib/i18n/useTranslation';
	import { cn } from '$lib/utils.js';
	import type { ActivityDay } from '$lib/types';

	interface Props {
		/** One entry per day of the window, zero days included. */
		days: ActivityDay[];
		/** Month starts to render, oldest first. */
		months: Date[];
		loading?: boolean;
	}

	let { days, months, loading = false }: Props = $props();
	const { t, locale } = useTranslation();

	const dateKey = new Intl.DateTimeFormat('en-CA'); // yields YYYY-MM-DD
	const dayLabel = $derived(
		new Intl.DateTimeFormat(locale.value, { day: 'numeric', month: 'short' })
	);
	const cellLabel = $derived(
		new Intl.DateTimeFormat(locale.value, { weekday: 'long', day: 'numeric', month: 'long' })
	);
	const monthLabel = $derived(
		new Intl.DateTimeFormat(locale.value, { month: 'short', year: 'numeric' })
	);
	// Weekday initials, Monday-first; only every other label fits the grid.
	const weekdayLabels = $derived.by(() => {
		const formatter = new Intl.DateTimeFormat(locale.value, { weekday: 'short' });
		const monday = new Date(2024, 0, 1); // a known Monday
		return Array.from({ length: 7 }, (_, index) =>
			formatter.format(new Date(monday.getFullYear(), monday.getMonth(), monday.getDate() + index))
		);
	});

	let counts = $derived(new Map(days.map((day) => [day.date, day.count])));
	let total = $derived(days.reduce((sum, day) => sum + day.count, 0));

	// One shared scale for every month, otherwise one chat in a quiet month would
	// look as dark as a busy day in another.
	let maxCount = $derived(days.reduce((max, day) => Math.max(max, day.count), 0));
	let step = $derived(maxCount <= 4 ? 1 : Math.ceil(maxCount / 4));

	function level(count: number) {
		if (count <= 0) return 0;
		return Math.min(4, Math.ceil(count / step));
	}

	const levelClasses = [
		'bg-muted',
		'bg-primary/20',
		'bg-primary/40',
		'bg-primary/65',
		'bg-primary/90'
	];

	interface MonthGrid {
		key: string;
		label: string;
		cells: (ActivityDay | null)[];
	}

	/** Pads each month to whole Monday-first weeks so grids line up. */
	let grids = $derived<MonthGrid[]>(
		months.map((month) => {
			const year = month.getFullYear();
			const monthIndex = month.getMonth();
			const offset = (new Date(year, monthIndex, 1).getDay() + 6) % 7;
			const dayCount = new Date(year, monthIndex + 1, 0).getDate();
			const cells: (ActivityDay | null)[] = Array.from({ length: offset }, () => null);
			for (let day = 1; day <= dayCount; day += 1) {
				const date = dateKey.format(new Date(year, monthIndex, day));
				cells.push({ date, count: counts.get(date) ?? 0 });
			}
			while (cells.length % 7 !== 0) cells.push(null);
			return { key: `${year}-${monthIndex}`, label: monthLabel.format(month), cells };
		})
	);

	let busiest = $derived(
		days.reduce<ActivityDay | null>(
			(best, day) => (!best || day.count > best.count ? day : best),
			null
		)
	);

	/** Consecutive days with at least one chat, counting back from today. */
	let streak = $derived.by(() => {
		let cursor = new Date();
		let count = 0;
		for (let index = 0; index <= days.length; index += 1) {
			const key = dateKey.format(cursor);
			if (!counts.has(key) || (counts.get(key) ?? 0) === 0) break;
			count += 1;
			cursor = new Date(cursor.getFullYear(), cursor.getMonth(), cursor.getDate() - 1);
		}
		return count;
	});

	function cellDate(date: string) {
		return dayLabel.format(new Date(`${date}T00:00:00`));
	}
</script>

{#if loading}
	<Skeleton class="h-full min-h-40 w-full" />
{:else}
	<div class="flex min-h-0 flex-1 flex-col gap-4">
		<div
			class="grid flex-1 content-start grid-cols-[repeat(auto-fit,minmax(150px,1fr))] gap-x-6 gap-y-5"
		>
			{#each grids as grid (grid.key)}
				<div class="min-w-0 space-y-2">
					<p class="text-[11px] font-medium text-muted-foreground">{grid.label}</p>
					<div class="flex gap-2">
						<div class="grid grid-rows-7 gap-1 text-[9px] text-muted-foreground">
							{#each weekdayLabels as label, row (row)}
								<span class="flex h-5 w-7 items-center">
									{row % 2 === 0 ? label : ''}
								</span>
							{/each}
						</div>
						<div class="grid grid-flow-col grid-rows-7 gap-1">
							{#each grid.cells as cell, cellIndex (cellIndex)}
								{#if cell}
									<div
										class={cn('size-5 rounded-sm', levelClasses[level(cell.count)])}
										title={`${cellLabel.format(
											new Date(`${cell.date}T00:00:00`)
										)} · ${t('myaaw.home.activity_cell', { count: cell.count })}`}
									></div>
								{:else}
									<div class="size-5"></div>
								{/if}
							{/each}
						</div>
					</div>
				</div>
			{/each}
		</div>

		<div class="flex shrink-0 flex-wrap items-end justify-between gap-3 border-t pt-3">
			<div class="flex items-baseline gap-3">
				<p class="text-2xl font-bold">{total}</p>
				<div class="space-y-0.5 text-[11px] text-muted-foreground">
					<p>{t('myaaw.home.activity_total', { count: total })}</p>
					{#if total > 0}
						<p>
							{#if busiest && busiest.count > 0}
								{t('myaaw.home.activity_busiest', {
									date: cellDate(busiest.date),
									count: busiest.count
								})}
							{/if}
							{#if streak > 1}
								· {t('myaaw.home.activity_streak', { count: streak })}
							{/if}
						</p>
					{:else}
						<p>{t('myaaw.home.activity_empty')}</p>
					{/if}
				</div>
			</div>

			<div class="flex items-center gap-1.5 text-[10px] text-muted-foreground">
				<span>{t('myaaw.home.activity_less')}</span>
				{#each levelClasses as levelClass (levelClass)}
					<span class={cn('h-3 w-3 rounded-sm', levelClass)}></span>
				{/each}
				<span>{t('myaaw.home.activity_more')}</span>
			</div>
		</div>
	</div>
{/if}
