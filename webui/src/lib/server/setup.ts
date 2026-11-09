import { chmodSync, existsSync, mkdirSync, readFileSync, renameSync, writeFileSync } from 'node:fs';
import { createHash, randomBytes, timingSafeEqual } from 'node:crypto';
import { dirname, join } from 'node:path';
import { arch, homedir, platform } from 'node:os';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';

const execFileAsync = promisify(execFile);

const REPO = 'Shiyinq/myaaw';
const DEFAULT_API = 'http://localhost:9000';
const SETUP_TOKEN_TTL_MS = 10 * 60 * 1000;

export interface SetupStatus {
	binaryInstalled: boolean;
	controlPlaneReachable: boolean;
	binaryPath: string;
}

interface ReleaseAsset {
	name: string;
	browser_download_url: string;
}

interface Release {
	assets?: ReleaseAsset[];
}

// A one-time token ties the install POST to the setup page that rendered it.
// It lives in process memory because the dashboard is a single local process.
let setupToken: { value: string; expiresAt: number } | null = null;

export function issueSetupToken(): string {
	if (setupToken && Date.now() <= setupToken.expiresAt) {
		return setupToken.value;
	}
	setupToken = {
		value: randomBytes(24).toString('hex'),
		expiresAt: Date.now() + SETUP_TOKEN_TTL_MS
	};
	return setupToken.value;
}

export function checkSetupToken(token: string | null | undefined): boolean {
	if (!setupToken || !token) return false;
	if (Date.now() > setupToken.expiresAt) {
		setupToken = null;
		return false;
	}
	const expected = Buffer.from(setupToken.value);
	const provided = Buffer.from(token);
	return expected.length === provided.length && timingSafeEqual(expected, provided);
}

export function consumeSetupToken(token: string | null | undefined): void {
	if (checkSetupToken(token)) {
		setupToken = null;
	}
}

function goos(): string | null {
	switch (platform()) {
		case 'darwin':
			return 'darwin';
		case 'linux':
			return 'linux';
		case 'win32':
			return 'windows';
		default:
			return null;
	}
}

function goarch(): string | null {
	switch (arch()) {
		case 'x64':
			return 'amd64';
		case 'arm64':
			return 'arm64';
		default:
			return null;
	}
}

export function binaryAssetName(): string | null {
	const os = goos();
	const cpu = goarch();
	if (!os || !cpu) return null;
	return `myaaw-${os}-${cpu}${os === 'windows' ? '.exe' : ''}`;
}

export function binaryPath(): string {
	if (platform() === 'win32') {
		const base = process.env.LOCALAPPDATA || join(homedir(), 'AppData', 'Local');
		return join(base, 'Programs', 'myaaw', 'myaaw.exe');
	}
	return join(homedir(), '.local', 'bin', 'myaaw');
}

export function isBinaryInstalled(): boolean {
	return existsSync(binaryPath());
}

// The dashboard's own config, written by the CLI and `webui-install.sh`.
function readWebUIConfig(): Record<string, unknown> | null {
	try {
		const path = join(homedir(), '.myaaw', 'webui', 'config.json');
		const parsed: unknown = JSON.parse(readFileSync(path, 'utf8'));
		return typeof parsed === 'object' && parsed !== null
			? (parsed as Record<string, unknown>)
			: null;
	} catch {
		return null;
	}
}

// webuiConfigApiUrl reads the control-plane URL written by `myaaw webui install
// --api-url` / `webui-install.sh --connect`, so the dashboard can pre-fill it.
export function webuiConfigApiUrl(): string {
	const parsed = readWebUIConfig();
	return typeof parsed?.api_url === 'string' ? parsed.api_url : '';
}

async function controlPlaneReachable(apiUrl: string): Promise<boolean> {
	try {
		const response = await fetch(`${apiUrl.replace(/\/+$/, '')}/health`, {
			signal: AbortSignal.timeout(1500)
		});
		return response.ok;
	} catch {
		return false;
	}
}

export async function setupStatus(apiUrl = DEFAULT_API): Promise<SetupStatus> {
	return {
		binaryInstalled: isBinaryInstalled(),
		controlPlaneReachable: await controlPlaneReachable(apiUrl),
		binaryPath: binaryPath()
	};
}

async function fetchRelease(): Promise<Release> {
	const response = await fetch(`https://api.github.com/repos/${REPO}/releases/latest`, {
		headers: {
			Accept: 'application/vnd.github+json',
			'User-Agent': 'myaaw-webui-setup'
		}
	});
	if (!response.ok) {
		throw new Error(`GitHub API returned ${response.status}`);
	}
	return (await response.json()) as Release;
}

function parseChecksums(text: string): Map<string, string> {
	const sums = new Map<string, string>();
	for (const line of text.split('\n')) {
		const match = line.trim().match(/^([0-9a-fA-F]{64})\s+\*?(.+)$/);
		if (match) {
			sums.set(match[2].trim(), match[1].toLowerCase());
		}
	}
	return sums;
}

async function download(url: string): Promise<Buffer> {
	const response = await fetch(url);
	if (!response.ok) {
		throw new Error(`Download failed (${response.status})`);
	}
	return Buffer.from(await response.arrayBuffer());
}

async function verifyChecksum(release: Release, asset: ReleaseAsset, data: Buffer): Promise<void> {
	const sumsAsset = release.assets?.find((entry) => entry.name === 'SHA256SUMS');
	if (!sumsAsset) {
		// Releases before the checksum file existed are installed as-is.
		return;
	}

	const response = await fetch(sumsAsset.browser_download_url);
	if (!response.ok) {
		throw new Error(`Checksum download failed (${response.status})`);
	}
	const expected = parseChecksums(await response.text()).get(asset.name);
	if (!expected) {
		throw new Error(`SHA256SUMS does not list ${asset.name}`);
	}

	const actual = createHash('sha256').update(data).digest('hex');
	if (actual !== expected) {
		throw new Error(`Checksum mismatch for ${asset.name}`);
	}
}

export async function installBinary(): Promise<string> {
	const assetName = binaryAssetName();
	if (!assetName) {
		throw new Error(`Unsupported platform: ${platform()}/${arch()}`);
	}

	const release = await fetchRelease();
	const asset = release.assets?.find((entry) => entry.name === assetName);
	if (!asset) {
		throw new Error(`No ${assetName} binary in the latest release`);
	}

	const data = await download(asset.browser_download_url);
	await verifyChecksum(release, asset, data);

	const target = binaryPath();
	mkdirSync(dirname(target), { recursive: true });
	const temp = `${target}.tmp`;
	writeFileSync(temp, data);
	if (platform() !== 'win32') {
		chmodSync(temp, 0o755);
	}
	renameSync(temp, target);
	return target;
}

export async function startControlPlane(apiUrl = DEFAULT_API): Promise<void> {
	let port = '9000';
	try {
		port = new URL(apiUrl).port || '9000';
	} catch {
		port = '9000';
	}
	await execFileAsync(binaryPath(), ['api', 'start', '--port', port], { env: process.env });
}
