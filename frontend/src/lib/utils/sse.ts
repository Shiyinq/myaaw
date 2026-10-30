import { API_BASE } from '$lib/constants';
import { accessToken } from '$lib/stores/accessToken.svelte';
import { toApiError } from '$lib/apis/client';

export interface SSEEvent {
	event: string;
	data: unknown;
}

interface StreamOptions {
	method?: string;
	body?: unknown;
	signal?: AbortSignal;
	onEvent: (event: SSEEvent) => void;
}

// EventSource cannot send a POST body or an Authorization header, so both chat
// streaming and log tailing go through fetch + ReadableStream.
export async function streamSSE(path: string, options: StreamOptions): Promise<void> {
	const token = accessToken.value;
	const response = await fetch(`${API_BASE}${path}`, {
		method: options.method ?? 'POST',
		headers: {
			'Content-Type': 'application/json',
			Accept: 'text/event-stream',
			...(token ? { Authorization: `Bearer ${token}` } : {})
		},
		body: options.body !== undefined ? JSON.stringify(options.body) : undefined,
		signal: options.signal
	});

	if (!response.ok) {
		throw await toApiError(response);
	}
	if (!response.body) {
		throw new Error('Streaming is not supported by this browser');
	}

	const reader = response.body.getReader();
	const decoder = new TextDecoder();
	let buffer = '';

	const handleBlock = (block: string) => {
		let event = 'message';
		const data: string[] = [];

		for (const line of block.split(/\r?\n/)) {
			if (line.startsWith('event:')) {
				event = line.slice(6).trim();
			} else if (line.startsWith('data:')) {
				data.push(line.slice(5).trimStart());
			}
		}

		if (data.length === 0) return;
		const raw = data.join('\n');
		let parsed: unknown = raw;
		try {
			parsed = JSON.parse(raw);
		} catch {
			// Keep the raw string when the payload is not JSON.
		}
		options.onEvent({ event, data: parsed });
	};

	while (true) {
		const { value, done } = await reader.read();
		if (done) break;
		buffer += decoder.decode(value, { stream: true });

		const blocks = buffer.split(/\r?\n\r?\n/);
		buffer = blocks.pop() ?? '';
		blocks.forEach(handleBlock);
	}

	buffer += decoder.decode();
	if (buffer.trim() !== '') {
		handleBlock(buffer);
	}
}
