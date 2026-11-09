import { browser } from '$app/environment';
import { API_BASE_KEY } from './auth';

// `/api` keeps the same-origin proxy (tokens never leave this origin). An
// absolute URL (loopback, Tailscale, Cloudflare, ngrok, or an SSH forward)
// makes the browser call the control plane directly.
export const DEFAULT_API_BASE = '/api';

export function normalizeApiBase(value: string): string {
	let trimmed = value.trim().replace(/\/+$/, '');
	// Be forgiving when someone pastes the proxied path.
	if (/^https?:\/\//i.test(trimmed)) {
		trimmed = trimmed.replace(/\/api$/i, '');
	}
	return trimmed === '' ? DEFAULT_API_BASE : trimmed;
}

let currentApiBase = browser
	? normalizeApiBase(localStorage.getItem(API_BASE_KEY) ?? '')
	: DEFAULT_API_BASE;

export function getApiBase(): string {
	return currentApiBase;
}

export function setApiBase(value: string): void {
	currentApiBase = normalizeApiBase(value);
	if (browser) {
		if (currentApiBase === DEFAULT_API_BASE) {
			localStorage.removeItem(API_BASE_KEY);
		} else {
			localStorage.setItem(API_BASE_KEY, currentApiBase);
		}
	}
}

// Only /health is unauthenticated on the control plane. Everything else needs
// the bearer token.
export const PUBLIC_ENDPOINTS = ['/health'];
