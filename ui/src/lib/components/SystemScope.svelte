<script lang="ts">
	import type { Snippet } from 'svelte';
	import { untrack } from 'svelte';
	import { LiveSystem } from '$lib/live.svelte';
	import { setLive } from '$lib/context';
	import { systems, findSystem } from '$lib/systems.svelte';
	import Empty from './Empty.svelte';

	let { id, children }: { id: string; children: Snippet } = $props();

	const live = new LiveSystem(untrack(() => id));
	setLive(live);
	const sys = $derived(findSystem(live.id));
	const monitored = $derived(!!sys?.status.monitored);

	$effect(() => {
		if (monitored) {
			live.start();
			return () => live.close();
		}
	});
</script>

{#if !sys && systems.loaded}
	<div class="page">
		<div class="card"><Empty icon="alert" title="System not found">There is no system with id <code>{live.id}</code>.</Empty></div>
	</div>
{:else}
	{@render children()}
{/if}
