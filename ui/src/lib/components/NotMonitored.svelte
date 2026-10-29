<script lang="ts">
	import { can } from '$lib/session.svelte';
	import Empty from './Empty.svelte';
	let { system }: { system: string } = $props();
</script>

<div class="card">
	<Empty icon="key" title="System account credentials needed">
		Topology, health and maintenance read from the NATS system account.
		{#if can('admin')}<div style="margin-top:12px"><a class="btn" href="/systems/{system}/settings">Add system account credentials</a></div>{/if}
	</Empty>
</div>
