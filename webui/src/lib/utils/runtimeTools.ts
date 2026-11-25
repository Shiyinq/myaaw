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

/**
 * Formats an external runtime model value for human display. Some ACP runtimes
 * (e.g. DeepSeek Harness) encode model IDs as JSON 2-tuples such as
 * `["deepseek-account","deepseek-flash"]`; render those as
 * `deepseek-account/deepseek-flash` while keeping the raw wire value for requests.
 */
export function formatRuntimeModelValue(raw?: string | null): string {
	const value = (raw ?? '').trim();
	if (!value) return '';
	if (value.startsWith('[') && value.endsWith(']')) {
		try {
			const parsed: unknown = JSON.parse(value);
			if (
				Array.isArray(parsed) &&
				parsed.length > 0 &&
				parsed.every((part) => typeof part === 'string' && part.trim() !== '')
			) {
				return (parsed as string[]).map((part) => part.trim()).join('/');
			}
		} catch {
			// Fall back to raw value.
		}
	}
	return value;
}

/**
 * Resolves a human-friendly label for an external runtime model ID, preferring
 * the runtime's advertised option label and falling back to `formatRuntimeModelValue`.
 */
export function formatRuntimeModelLabel(
	raw: string | null | undefined,
	options?: ReadonlyArray<{ value: string; label: string }>
): string {
	const value = (raw ?? '').trim();
	if (!value) return '';
	const match = options?.find((item) => item.value === value);
	if (match?.label) return match.label;
	return formatRuntimeModelValue(value);
}
