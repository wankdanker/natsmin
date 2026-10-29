<script lang="ts">
	import { page } from '$app/state';
	import { findSystem } from '$lib/systems.svelte';
	import Empty from '$lib/components/Empty.svelte';

	let { children } = $props();
	const sys = $derived(findSystem(page.params.system!));
	const acc = $derived(sys?.accounts.find((a) => a.id === page.params.account));
</script>

{#if sys && !acc}
	<div class="page">
		<div class="card"><Empty icon="alert" title="Account connection not found">{page.params.account} is not configured for {sys.name}.</Empty></div>
	</div>
{:else}
	{@render children()}
{/if}
