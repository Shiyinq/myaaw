import { skills as skillsApi } from '$lib/apis/skills';
import type { SkillDetail, SkillFileContent, SkillsResponse } from '$lib/types';
import { createQueryStore } from './query.svelte';

const list = createQueryStore<{ agent: string }, SkillsResponse>({
	key: (args) => `skills:list:${args.agent}`,
	fetcher: (args) => skillsApi.list(args.agent)
});

const detail = createQueryStore<{ agent: string; name: string; builtin: boolean }, SkillDetail>({
	key: (args) => `skills:detail:${args.agent}:${args.builtin ? 'builtin' : 'agent'}:${args.name}`,
	fetcher: (args) =>
		args.builtin ? skillsApi.builtin(args.name) : skillsApi.get(args.agent, args.name)
});

const file = createQueryStore<
	{ agent: string; name: string; path: string; builtin: boolean },
	SkillFileContent
>({
	key: (args) =>
		`skills:file:${args.agent}:${args.builtin ? 'builtin' : 'agent'}:${args.name}:${args.path}`,
	fetcher: (args) =>
		args.builtin
			? skillsApi.builtinFile(args.name, args.path)
			: skillsApi.file(args.agent, args.name, args.path)
});

function patchSkill(name: string, enabled: boolean) {
	const current = list.data;
	if (current) {
		list.setData((previous) =>
			previous
				? {
						skills: previous.skills.map((skill) =>
							skill.name === name ? { ...skill, enabled } : skill
						)
					}
				: previous
		);
	}
	const cached = detail.data;
	if (cached?.name === name && !detail.args?.builtin) {
		detail.setData((previous) => (previous ? { ...previous, enabled } : previous));
	}
}

export const skillsStore = {
	list,
	detail,
	file,
	get data() {
		return list.data;
	},
	get loading() {
		return list.loading;
	},
	get refreshing() {
		return list.refreshing;
	},
	get error() {
		return list.error;
	},
	get status() {
		return list.status;
	},
	ensure(agent: string) {
		return list.ensure({ agent });
	},
	fetchList(agent: string) {
		return skillsApi.list(agent);
	},
	refresh() {
		return list.refresh();
	},
	ensureDetail(agent: string, name: string, builtin = false) {
		return detail.ensure({ agent, name, builtin });
	},
	/** One-shot read for a dialog that keeps its own copy. */
	fetchDetail(agent: string, name: string) {
		return skillsApi.get(agent, name);
	},
	ensureFile(agent: string, name: string, path: string, builtin = false) {
		return file.ensure({ agent, name, path, builtin });
	},
	/** One-shot read for a dialog that must not disturb the active query. */
	fetchFile(agent: string, name: string, path: string) {
		return skillsApi.file(agent, name, path);
	},
	async install(agent: string, name: string) {
		const installed = await skillsApi.install(agent, name);
		await list.refresh({ agent });
		detail.write({ agent, name, builtin: false }, installed);
		return installed;
	},
	async toggle(agent: string, name: string, enabled: boolean) {
		patchSkill(name, enabled);
		try {
			await skillsApi.toggle(agent, name, enabled);
		} catch (error) {
			await list.refresh({ agent });
			throw error;
		}
	},
	async remove(agent: string, name: string) {
		await skillsApi.remove(agent, name);
		detail.reset({ agent, name, builtin: false });
		await list.refresh({ agent });
	},
	async removeConfig(agent: string, name: string) {
		await skillsApi.removeConfig(agent, name);
		detail.invalidate({ agent, name, builtin: false });
		await list.refresh({ agent });
	}
};
