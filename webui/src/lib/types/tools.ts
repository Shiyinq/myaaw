import type { SecretState } from './config';

export interface BuiltinTool {
	name: string;
	description?: string;
	enabled: boolean;
}

export interface CustomTool {
	name: string;
	command: string;
	args?: string[];
	schema?: unknown;
	enabled: boolean;
}

export interface MCPServer {
	name: string;
	command: string;
	args?: string[];
	enabled: boolean;
	env: SecretState;
}

export interface ToolsView {
	builtin: BuiltinTool[];
	custom: CustomTool[];
	mcp_servers: MCPServer[];
}

export interface CustomToolInput {
	name: string;
	command: string;
	args?: string[];
	schema?: unknown;
	enabled?: boolean;
}

export interface MCPServerInput {
	name: string;
	command: string;
	args?: string[];
	env?: Record<string, string>;
	enabled?: boolean;
}
