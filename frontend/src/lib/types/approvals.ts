import type { ApprovalsConfig } from './config';

export interface GrantScope {
	channel?: string;
	user_id?: string;
}

export interface ApprovalGrant {
	key: string;
	always?: GrantScope[];
	sessions?: {
		conversation: string;
		channel?: string;
		user_id?: string;
	}[];
}

export interface ApprovalsView {
	config: ApprovalsConfig;
	grants: ApprovalGrant[] | null;
}
