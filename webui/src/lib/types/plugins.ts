import type { BuiltinTool, CustomTool, MCPServer, Skill } from '$lib/types';

/** The four groups the plugins page lists. */
export type PluginKind = 'skill' | 'builtin' | 'custom' | 'mcp';

/** One entry, resolved against the list the page is currently showing. */
export type PluginTarget =
	| { kind: 'skill'; skill: Skill }
	| { kind: 'builtin'; tool: BuiltinTool }
	| { kind: 'custom'; tool: CustomTool }
	| { kind: 'mcp'; server: MCPServer };
