import {
	conversations as conversationsApi,
	type AttachmentFilters,
	type ConversationFilters,
	type ProjectListFilters
} from '$lib/apis/conversations';
import type {
	ConversationActivity,
	ConversationAttachmentPage,
	ConversationDetail,
	ConversationPage,
	ConversationProjectsResponse,
	InteractionRequest
} from '$lib/types';
import { createQueryStore } from './query.svelte';

const list = createQueryStore<{ agent: string; filters: ConversationFilters }, ConversationPage>({
	key: (args) => `conversations:list:${args.agent}:${JSON.stringify(args.filters)}`,
	fetcher: (args) => conversationsApi.list(args.agent, args.filters)
});

const detail = createQueryStore<
	{ agent: string; id: string; limit?: number; offset?: number },
	ConversationDetail
>({
	key: (args) =>
		`conversations:detail:${args.agent}:${args.id}:${args.limit ?? ''}:${args.offset ?? ''}`,
	fetcher: (args) =>
		conversationsApi.get(args.agent, args.id, { limit: args.limit, offset: args.offset })
});

const attachments = createQueryStore<
	{ agent: string; id: string; filters: AttachmentFilters },
	ConversationAttachmentPage
>({
	key: (args) =>
		`conversations:attachments:${args.agent}:${args.id}:${JSON.stringify(args.filters)}`,
	fetcher: (args) => conversationsApi.attachments(args.agent, args.id, args.filters)
});

const activity = createQueryStore<
	{ agent: string; from: number; to: number },
	ConversationActivity
>({
	key: (args) => `conversations:activity:${args.agent}:${args.from}:${args.to}`,
	fetcher: (args) => conversationsApi.activity(args.agent, { from: args.from, to: args.to })
});

const projects = createQueryStore<{ agent: string }, ConversationProjectsResponse>({
	key: (args) => `conversations:projects:${args.agent}`,
	fetcher: (args) => conversationsApi.projects(args.agent)
});

export const conversationsStore = {
	list,
	detail,
	attachments,
	activity,
	projects,
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
	ensureList(agent: string, filters: ConversationFilters = {}) {
		return list.ensure({ agent, filters });
	},
	ensureDetail(agent: string, id: string, limit?: number, offset?: number) {
		return detail.ensure({ agent, id, limit, offset });
	},
	/** Paged reads that a component accumulates locally. */
	fetchList(agent: string, filters: ConversationFilters = {}) {
		return conversationsApi.list(agent, filters);
	},
	fetchDetail(agent: string, id: string, params: { limit?: number; offset?: number } = {}) {
		return conversationsApi.get(agent, id, params);
	},
	fetchAttachments(agent: string, id: string, filters: AttachmentFilters = {}) {
		return conversationsApi.attachments(agent, id, filters);
	},
	ensureAttachments(agent: string, id: string, filters: AttachmentFilters = {}) {
		return attachments.ensure({ agent, id, filters });
	},
	ensureActivity(agent: string, from: number, to: number) {
		return activity.ensure({ agent, from, to });
	},
	fetchProjects(agent: string, filters: ProjectListFilters = {}) {
		return conversationsApi.projects(agent, filters);
	},
	ensureProjects(agent: string) {
		return projects.ensure({ agent });
	},
	create(
		agent: string,
		payload: { title?: string; working_dir?: string; folder_id?: string } = {}
	) {
		return conversationsApi.create(agent, payload);
	},
	updateWorkingDir(agent: string, id: string, workingDir: string) {
		return conversationsApi.updateWorkingDir(agent, id, workingDir);
	},
	respondInteraction(agent: string, id: string, payload: InteractionRequest) {
		return conversationsApi.respondInteraction(agent, id, payload);
	},
	/** Mutations invalidate every cached view of that conversation. */
	invalidate() {
		list.invalidate();
		detail.invalidate();
		attachments.invalidate();
		activity.invalidate();
		projects.invalidate();
	}
};
