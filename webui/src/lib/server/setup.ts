import { chmodSync, existsSync, mkdirSync, renameSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { arch, homedir, platform } from 'node:os';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';

const execFileAsync = promisify(execFile);

const REPO = 'Shiyinq/myaaw';
const DEFAULT_API = 'http://localhost:9000';

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

	const response = await fetch(asset.browser_download_url);
	if (!response.ok) {
		throw new Error(`Binary download failed (${response.status})`);
	}

	const target = binaryPath();
	mkdirSync(dirname(target), { recursive: true });
	const temp = `${target}.tmp`;
	writeFileSync(temp, Buffer.from(await response.arrayBuffer()));
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
