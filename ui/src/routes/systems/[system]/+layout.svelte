<script lang="ts">
	import { page } from '$app/state';
	import SystemScope from '$lib/components/SystemScope.svelte';

	let { children } = $props();
</script>

{#key page.params.system}
	<SystemScope id={page.params.system!}>{@render children()}</SystemScope>
{/key}
