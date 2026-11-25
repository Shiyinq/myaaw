/**
 * Built-in Myaaw tools delegated directly to the external CLI runtime's own
 * native tools (and intentionally excluded from `myaaw mcp-bridge`).
 * Keep in sync with `CoreCodingTools` in internal/agent/mcpbridge/bridge.go.
 */
export const EXTERNAL_DELEGATED_TOOLS = new Set([
	'filesystem',
	'bash',
	'python',
	'execute_python',
	'subagent',
	'provider'
]);

/** Returns true when runtimeType selects an external CLI engine instead of native Myaaw. */
export function isExternalRuntimeType(runtimeType?: string | null): boolean {
	const value = (runtimeType ?? '').trim().toLowerCase();
	return value !== '' && value !== 'myaaw';
}

/**
 * Returns true when a built-in Myaaw tool applies to the chosen runtime.
 * Native `myaaw` agents expose every built-in tool; external CLI agents only
 * receive Myaaw ecosystem tools through `myaaw mcp-bridge`.
 */
export function isToolAvailableForRuntime(toolName: string, isExternal: boolean): boolean {
	if (!isExternal) return true;
	return !EXTERNAL_DELEGATED_TOOLS.has(toolName.trim().toLowerCase());
}
