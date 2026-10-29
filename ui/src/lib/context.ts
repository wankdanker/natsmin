import { getContext, setContext } from 'svelte';
import type { LiveSystem } from './live.svelte';

const key = Symbol('live-system');

export function setLive(l: LiveSystem) {
	setContext(key, l);
}

/** The live system of the current /systems/[system] route. */
export function getLive(): LiveSystem {
	return getContext<LiveSystem>(key);
}
