import { t } from '$lib/i18n';

interface PageTitle {
	/** Translation key of the name shown in the browser tab. */
	titleKey: string;
	/** Translation key of the description the page advertises. */
	descriptionKey: string;
	/** The key already reads as a whole document title, so no brand prefix. */
	standalone?: boolean;
}

/**
 * One entry per destination. Keeping this in one place means the tab always
 * names the page the user is on, instead of falling back to whatever title the
 * previous route left in the document.
 */
const pageTitles: Record<string, PageTitle> = {
	'/': {
		titleKey: 'landing.meta.title',
		descriptionKey: 'landing.meta.description',
		standalone: true
	},
	'/login': { titleKey: 'common.login', descriptionKey: 'myaaw.login.subtitle' },
	'/offline': { titleKey: 'offline.title', descriptionKey: 'offline.description' },
	'/app': { titleKey: 'myaaw.nav.fleet', descriptionKey: 'myaaw.fleet.subtitle' },
	'/app/dashboard': { titleKey: 'myaaw.nav.dashboard', descriptionKey: 'myaaw.home.subtitle' },
	'/app/chat': { titleKey: 'myaaw.nav.chat', descriptionKey: 'myaaw.chat.subtitle' },
	'/app/context': { titleKey: 'myaaw.nav.context', descriptionKey: 'myaaw.context.subtitle' },
	'/app/skills': { titleKey: 'myaaw.nav.skills', descriptionKey: 'myaaw.skills.subtitle' },
	'/app/tools': { titleKey: 'myaaw.nav.tools', descriptionKey: 'myaaw.tools.subtitle' },
	'/app/contacts': { titleKey: 'myaaw.nav.contacts', descriptionKey: 'myaaw.contacts.subtitle' },
	'/app/files': { titleKey: 'myaaw.nav.files', descriptionKey: 'myaaw.files.subtitle' },
	'/app/terminal': { titleKey: 'myaaw.nav.terminal', descriptionKey: 'myaaw.terminal.subtitle' },
	'/app/providers': { titleKey: 'myaaw.nav.providers', descriptionKey: 'myaaw.providers.subtitle' },
	'/app/channels': { titleKey: 'myaaw.nav.channels', descriptionKey: 'myaaw.channels.subtitle' },
	'/app/config': { titleKey: 'myaaw.nav.config', descriptionKey: 'myaaw.config.subtitle' },
	'/app/cron': { titleKey: 'myaaw.nav.cron', descriptionKey: 'myaaw.cron.subtitle' },
	'/app/server': { titleKey: 'myaaw.nav.server', descriptionKey: 'myaaw.server.subtitle' },
	'/app/logs': { titleKey: 'myaaw.nav.logs', descriptionKey: 'myaaw.logs.subtitle' },
	'/app/backups': { titleKey: 'myaaw.nav.backups', descriptionKey: 'myaaw.backups.subtitle' },
	'/app/approvals': { titleKey: 'myaaw.nav.approvals', descriptionKey: 'myaaw.approvals.subtitle' },
	'/app/observability': {
		titleKey: 'myaaw.nav.observability',
		descriptionKey: 'myaaw.observability.subtitle'
	},
	'/app/plugins': { titleKey: 'myaaw.plugins.title', descriptionKey: 'myaaw.plugins.subtitle' },
	'/app/new': { titleKey: 'myaaw.onboard.title', descriptionKey: 'myaaw.onboard.subtitle' },
	'/app/profile': { titleKey: 'myaaw.nav.profile', descriptionKey: 'myaaw.profile.subtitle' },
	'/app/settings': { titleKey: 'myaaw.nav.settings', descriptionKey: 'myaaw.settings.subtitle' }
};

/** Looks up the page a path belongs to, including its sub-pages. */
export function pageTitleFor(pathname: string): PageTitle | null {
	const direct = pageTitles[pathname];
	if (direct) return direct;

	// A conversation, a created resource and other sub-pages keep the title of
	// the section they live under.
	const segments = pathname.split('/').filter(Boolean);
	if (segments.length < 2) return null;
	return pageTitles[`/${segments[0]}/${segments[1]}`] ?? null;
}

/** The document title for a path, translated for the current locale. */
export function documentTitle(pathname: string): string {
	const meta = pageTitleFor(pathname);
	if (!meta) return 'Myaaw';
	const label = t(meta.titleKey);
	return meta.standalone ? label : `Myaaw | ${label}`;
}

/** The meta description for a path, translated for the current locale. */
export function documentDescription(pathname: string): string | undefined {
	const meta = pageTitleFor(pathname);
	return meta ? t(meta.descriptionKey) : undefined;
}
