import { api, unwrap, type Schemas } from './api/client';

/** Registered systems, shared by the sidebar and pages. */
export const systems = $state<{ list: Schemas['SystemOut'][]; loaded: boolean }>({ list: [], loaded: false });

export async function refreshSystems() {
	systems.list = await unwrap(api.GET('/v1/systems'));
	systems.loaded = true;
}

export function findSystem(id: string) {
	return systems.list.find((s) => s.id === id);
}
