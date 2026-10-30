<script lang="ts">
	import { page } from '$app/state';

	interface Props {
		title?: string;
		description?: string;
		image?: string;
		canonical?: string;
		type?: 'website' | 'article';
	}

	let {
		title = 'Myaaw Dashboard',
		description = 'Manage your Myaaw agents, chats, and channels from one control plane.',
		image = '/og-image.png',
		canonical = page.url.href,
		type = 'website'
	}: Props = $props();

	const siteName = 'Myaaw';
	const twitterHandle = '@myaaw';
</script>

<svelte:head>
	<!-- Primary Meta Tags -->
	<title>{title}</title>
	<meta name="title" content={title} />
	<meta name="description" content={description} />
	<link rel="canonical" href={canonical} />

	<!-- Open Graph / Facebook -->
	<meta property="og:type" content={type} />
	<meta property="og:url" content={page.url.href} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:image" content={image} />
	<meta property="og:site_name" content={siteName} />

	<!-- Twitter -->
	<meta property="twitter:card" content="summary_large_image" />
	<meta property="twitter:url" content={page.url.href} />
	<meta property="twitter:title" content={title} />
	<meta property="twitter:description" content={description} />
	<meta property="twitter:image" content={image} />
	<meta property="twitter:site" content={twitterHandle} />
</svelte:head>
