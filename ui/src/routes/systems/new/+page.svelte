<script lang="ts">
	import { goto } from '$app/navigation';
	import { refreshSystems } from '$lib/systems.svelte';
	import { toast } from '$lib/toast.svelte';
	import SystemForm from '$lib/components/SystemForm.svelte';
</script>

<svelte:head><title>Add system · natsmin</title></svelte:head>

<div class="page" style="max-width: 980px">
	<div class="page-head">
		<div>
			<h1>Add a NATS system</h1>
			<div class="sub">A single server, a cluster, or a supercluster joined by gateways.</div>
		</div>
	</div>
	<SystemForm
		onsaved={async (s) => {
			await refreshSystems();
			toast(`Added ${s.name}`);
			goto(`/systems/${s.id}`);
		}}
	/>
</div>
