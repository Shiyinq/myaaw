export const API_BASE = '/api';

// Only /health is unauthenticated on the control plane. Everything else needs
// the bearer token.
export const PUBLIC_ENDPOINTS = ['/health'];
