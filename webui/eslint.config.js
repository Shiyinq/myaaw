import js from '@eslint/js';
import ts from 'typescript-eslint';
import svelte from 'eslint-plugin-svelte';
import prettier from 'eslint-config-prettier';
import globals from 'globals';
import svelteParser from 'svelte-eslint-parser';
import svelteConfig from './svelte.config.js';

/** @type {import('eslint').Linter.FlatConfig[]} */
export default [
	js.configs.recommended,
	...ts.configs.recommended,
	...svelte.configs['flat/recommended'],
	prettier,
	...svelte.configs['flat/prettier'],
	{
		rules: {
			'@typescript-eslint/no-unused-vars': [
				'error',
				{
					argsIgnorePattern: '^_',
					varsIgnorePattern: '^_',
					caughtErrorsIgnorePattern: '^_'
				}
			],
			'@typescript-eslint/no-explicit-any': 'off', // Relax this as per general project preference if needed, or keep for quality
			'svelte/valid-compile': 'off'
		}
	},
	{
		languageOptions: {
			globals: {
				...globals.browser,
				...globals.node
			}
		}
	},
	{
		files: ['**/*.svelte'],
		languageOptions: {
			parser: svelteParser,
			parserOptions: {
				parser: ts.parser,
				extraFileExtensions: ['.svelte'],
				svelteConfig,
				svelteFeatures: {
					runes: true
				}
			}
		},
		rules: {
			// Components talk to stores, never to the HTTP layer. A page that needs
			// new data adds a query store instead of importing `lib/apis` directly.
			'no-restricted-imports': [
				'error',
				{
					patterns: [
						{
							group: ['$lib/apis/*'],
							message:
								'Import the matching store from $lib/stores instead of calling the API directly.'
						}
					]
				}
			]
		}
	},
	{
		ignores: ['build/', '.svelte-kit/', 'dist/']
	}
];
