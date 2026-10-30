import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';
import tailwindcss from '@tailwindcss/vite';
import { SvelteKitPWA } from '@vite-pwa/sveltekit';

export default defineConfig({
	plugins: [
		tailwindcss(),
		sveltekit(),
		SvelteKitPWA({
			registerType: 'prompt',
			strategies: 'injectManifest',
			srcDir: 'src',
			filename: 'service-worker.ts',
			injectRegister: 'auto',
			devOptions: {
				enabled: false,
				suppressWarnings: true,
				type: 'module'
			},
			manifest: {
				name: 'Myaaw Dashboard',
				short_name: 'Myaaw',
				description: 'Control plane for Myaaw agents',
				theme_color: '#ffffff',
				background_color: '#ffffff',
				display: 'standalone',
				icons: [
					{
						src: '/logo-192x192.png',
						sizes: '192x192',
						type: 'image/png'
					},
					{
						src: '/logo-512x512.png',
						sizes: '512x512',
						type: 'image/png'
					}
				]
			},
			injectManifest: {
				globPatterns: ['client/**/*.{js,css,ico,png,svg,webp,webmanifest}', 'prerendered/**/*.html']
			}
		})
	],
	build: {
		sourcemap: false
	}
});
