import { client } from './client';
import type { ModelCapabilitiesView } from '$lib/types';

export const models = {
	/** Global capability catalog served from ~/.myaaw/config/model_capabilities.json. */
	capabilities: () => client<ModelCapabilitiesView>('/models/capabilities')
};
