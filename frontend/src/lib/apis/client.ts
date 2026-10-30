import { browser } from '$app/environment';
import { goto } from '$app/navigation';
import { API_BASE } from '$lib/constants';
import { accessToken } from '$lib/stores/accessToken.svelte';
import { isAuthenticated } from '$lib/stores/authStatus.svelte';
import type { ApiError } from '$lib/types';

export interface ClientOptions extends Omit<RequestInit, 'body'> {
	body?: unknown;
	responseType?: 'json' | 'text';
}

export async function toApiError(response: Response): Promise<ApiError> {
	const contentType = response.headers.get('content-type') ?? '';
	let payload: unknown = null;

	if (contentType.includes('application/json')) {
		payload = await response.json().catch(() => null);
	} else {
		payload = await response.text().catch(() => '');
	}

	if (typeof payload === 'object' && payload !== null) {
		return { ...(payload as Record<string, unknown>), status: response.status } as ApiError;
	}

	return {
		detail: typeof payload === 'string' && payload !== '' ? payload : response.statusText,
		status: response.status
	};
}

function authHeaders(): Record<string, string> {
	const token = accessToken.value;
	return token ? { Authorization: `Bearer ${token}` } : {};
}

function isRawBody(body: unknown): body is BodyInit {
	return (
		typeof body === 'string' ||
		(typeof FormData !== 'undefined' && body instanceof FormData) ||
		(typeof URLSearchParams !== 'undefined' && body instanceof URLSearchParams) ||
		(typeof Blob !== 'undefined' && body instanceof Blob) ||
		(typeof ArrayBuffer !== 'undefined' && body instanceof ArrayBuffer)
	);
}

export async function request(path: string, options: ClientOptions = {}): Promise<Response> {
	const headers = new Headers(options.headers);

	for (const [key, value] of Object.entries(authHeaders())) {
		headers.set(key, value);
	}

	const body = options.body;
	let payload: BodyInit | null | undefined;
	if (body === undefined || body === null) {
		payload = body;
	} else if (isRawBody(body)) {
		payload = body;
	} else {
		headers.set('Content-Type', 'application/json');
		payload = JSON.stringify(body);
	}

	const response = await fetch(`${API_BASE}${path}`, {
		method: options.method,
		headers,
		body: payload,
		signal: options.signal,
		credentials: options.credentials,
		cache: options.cache,
		redirect: options.redirect,
		referrer: options.referrer,
		referrerPolicy: options.referrerPolicy,
		integrity: options.integrity,
		keepalive: options.keepalive,
		mode: options.mode
	});

	if (response.status === 401) {
		accessToken.clear();
		isAuthenticated.set(false);
		if (browser && !location.pathname.startsWith('/login')) {
			void goto('/login');
		}
		throw await toApiError(response);
	}

	return response;
}

export async function client<T>(path: string, options: ClientOptions = {}): Promise<T> {
	const response = await request(path, options);

	if (!response.ok) {
		throw await toApiError(response);
	}
	if (response.status === 204) {
		return undefined as T;
	}
	if (options.responseType === 'text') {
		return (await response.text()) as T;
	}

	const contentType = response.headers.get('content-type') ?? '';
	if (contentType.includes('application/json')) {
		return (await response.json()) as T;
	}
	return (await response.text()) as T;
}

export function agentPath(name: string, suffix: string): string {
	return `/agents/${encodeURIComponent(name)}${suffix}`;
}
