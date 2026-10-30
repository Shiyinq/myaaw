import { dev } from '$app/environment';
import { addToast } from '$lib/stores';
import { getErrorMessage } from '$lib/utils/api';

type LogLevel = 'info' | 'warn' | 'error';

interface LogOptions {
	context?: string;
	showToast?: boolean;
	toastMessage?: string;
}

class Logger {
	private log(level: LogLevel, message: string, error?: unknown, options?: LogOptions) {
		const context = options?.context ? `[${options.context}]` : '';
		const timestamp = new Date().toISOString();

		// In development, log everything to console
		if (dev) {
			const consoleMethod =
				level === 'info' ? console.log : level === 'warn' ? console.warn : console.error;
			consoleMethod(`${timestamp} ${level.toUpperCase()} ${context} ${message}`, error ?? '');
		} else {
			// In production, only log errors — avoid leaking sensitive info
			// Uncomment below to send to a monitoring service (Sentry, etc.)
			// if (level === 'error') { Sentry.captureException(error); }
		}

		// Optional user feedback via toast
		if (options?.showToast) {
			const toastType = level === 'error' ? 'error' : 'success';
			let toastMsg = options.toastMessage;

			if (!toastMsg) {
				if (level === 'error') {
					toastMsg = error ? getErrorMessage(error) : 'An unexpected error occurred';
				} else {
					toastMsg = message;
				}
			}

			addToast(toastMsg, toastType);
		}
	}

	info(message: string, options?: LogOptions) {
		this.log('info', message, undefined, options);
	}

	warn(message: string, error?: unknown, options?: LogOptions) {
		this.log('warn', message, error, options);
	}

	error(message: string, error?: unknown, options?: LogOptions) {
		this.log('error', message, error, options);
	}
}

export const logger = new Logger();
