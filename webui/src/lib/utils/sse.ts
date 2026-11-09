import { API_BASE } from '$lib/constants';
import { accessToken } from '$lib/stores/accessToken.svelte';
import { toApiError } from '$lib/apis/client';

export interface SSEEvent {
	event: string;
	data: unknown;
}

export interface StreamRetryPolicy {
	/** Maximum number of automatic reconnects after the first attempt. */
	attempts: number;
	baseDelayMs?: number;
	maxDelayMs?: number;
	/** Called before each reconnect; use it to reset any UI state. */
	onRetry?: (attempt: number, error: unknown) => void;
	/** Return false to stop retrying, e.g. once a run has already started. */
	shouldRetry?: (attempt: number, error: unknown) => boolean;
}

interface StreamOptions {
	method?: string;
	body?: unknown;
	signal?: AbortSignal;
	onEvent: (event: SSEEvent) => void;
	retry?: StreamRetryPolicy;
}

function isAbort(error: unknown, signal?: AbortSignal) {
	return signal?.aborted === true || (error as Error | undefined)?.name === 'AbortError';
}

function isRetryable(error: unknown) {
	const status = (error as { status?: number } | undefined)?.status;
	// Client errors are terminal; network failures and 5xx are worth retrying.
	return typeof status !== 'number' || status >= 500;
}

function wait(ms: number) {
	return new Promise((resolve) => setTimeout(resolve, ms));
}

// EventSource cannot send a POST body or an Authorization header, so both chat
// streaming and log tailing go through fetch + ReadableStream.
export async function streamSSE(path: string, options: StreamOptions): Promise<void> {
	const retry = options.retry;
	let attempt = 0;

	for (;;) {
		try {
			await readStream(path, options);
			return;
		} catch (error) {
			if (isAbort(error, options.signal) || !isRetryable(error)) throw error;
			if (!retry || attempt >= retry.attempts) throw error;
			if (retry.shouldRetry && !retry.shouldRetry(attempt + 1, error)) throw error;

			attempt += 1;
			const backoff = (retry.baseDelayMs ?? 1000) * 2 ** (attempt - 1);
			retry.onRetry?.(attempt, error);
			await wait(Math.min(retry.maxDelayMs ?? 8000, backoff));
			if (options.signal?.aborted) throw error;
		}
	}
}

async function readStream(path: string, options: StreamOptions): Promise<void> {
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
