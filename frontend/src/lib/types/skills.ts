export interface Skill {
	name: string;
	display_name: string;
	description: string;
	enabled: boolean;
	source: string;
	parse_ok: boolean;
}

export interface SkillsResponse {
	skills: Skill[];
}

export interface SkillFile {
	path: string;
	size: number;
	mod_time?: string;
}

export interface SkillDetail extends Skill {
	content: string;
	files: SkillFile[];
}

export interface SkillFileContent {
	path: string;
	size: number;
	mod_time?: string;
	binary: boolean;
	content: string;
}
