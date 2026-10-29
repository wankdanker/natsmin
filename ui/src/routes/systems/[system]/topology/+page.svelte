<script lang="ts">
	import { getLive } from '$lib/context';
	import { findSystem } from '$lib/systems.svelte';
	import { bytes, compact, num, rate } from '$lib/format';
	import Topology from '$lib/components/Topology.svelte';
	import Loading from '$lib/components/Loading.svelte';
	import NotMonitored from '$lib/components/NotMonitored.svelte';
	import Icon from '$lib/components/Icon.svelte';

	const live = getLive();
	const sys = $derived(findSystem(live.id));
	const snap = $derived(live.snapshot);
	let selected = $state<string | null>(null);
	const server = $derived(snap?.servers.find((s) => s.id === selected));
	const routes = $derived(snap?.routes.filter((r) => r.from === selected) ?? []);
	const gws = $derived(snap?.gateways.filter((g) => g.from === selected) ?? []);
	const leafs = $derived(snap?.leafnodes.filter((l) => l.server === selected) ?? []);
	const nameOf = (id: string) => snap?.servers.find((s) => s.id === id)?.name ?? id.slice(0, 8);
</script>

<svelte:head><title>Topology · {sys?.name ?? live.id} · natsmin</title></svelte:head>

<div class="page" style="max-width:none">
	<div class="page-head">
		<div>
			<h1>Topology</h1>
			<div class="sub">Clusters, routes, gateways and leafnodes as the servers report them. Click a server for details.</div>
		</div>
	</div>
	{#if sys && !sys.status.monitored}
		<NotMonitored system={live.id} />
	{:else if !snap}
		<Loading />
	{:else}
		<div class="layout">
			<div class="card"><div class="card-body"><Topology snapshot={snap} bind:selected height={640} /></div></div>
			<div class="card side">
				{#if server}
					<div class="card-head">
						<span class="dot {server.missing ? '' : server.healthy ? 'ok' : 'bad'}"></span>
						<h2 class="ellipsis">{server.name}</h2>
						<span class="spacer"></span>
						<a class="btn sm" href="/systems/{live.id}/servers/{server.id}">Open<Icon name="chevronRight" size={14} /></a>
					</div>
					<div class="card-body stack">
						<dl class="kv-list">
							<dt>Cluster</dt><dd>{server.cluster || '—'}</dd>
							<dt>Version</dt><dd>{server.version}</dd>
							<dt>Host</dt><dd class="mono">{server.host}:{server.port}</dd>
							<dt>Uptime</dt><dd>{server.uptime}</dd>
							<dt>CPU / Mem</dt><dd>{server.cpu.toFixed(1)}% · {bytes(server.mem)}</dd>
							<dt>Connections</dt><dd class="num">{num(server.connections)}</dd>
							<dt>Msgs in/out</dt><dd class="num">{rate(server.inMsgsRate)} / {rate(server.outMsgsRate)}</dd>
							{#if server.tags?.length}<dt>Tags</dt><dd>{#each server.tags as t}<span class="badge">{t}</span> {/each}</dd>{/if}
							{#if !server.healthy}<dt>Health</dt><dd class="bad-text">{server.healthError || 'unhealthy'}</dd>{/if}
						</dl>
						{#if routes.length}
							<div>
								<h3>Routes</h3>
								<table class="t compact">
									<tbody>
										{#each routes as r}
											<tr><td>→ {r.toName || nameOf(r.to)}</td><td class="faint">{r.connections} conn</td><td class="num r">{r.rtt}</td></tr>
										{/each}
									</tbody>
								</table>
							</div>
						{/if}
						{#if gws.length}
							<div>
								<h3>Gateways</h3>
								<table class="t compact">
									<tbody>
										{#each gws as g}
											<tr>
												<td>→ {g.toCluster}</td>
												<td>{#if g.connected}<span class="badge ok">connected</span>{:else}<span class="badge bad">down</span>{/if}</td>
												<td class="num r">{g.rtt ?? ''}</td>
											</tr>
										{/each}
									</tbody>
								</table>
							</div>
						{/if}
						{#if leafs.length}
							<div>
								<h3>Leafnodes</h3>
								<table class="t compact">
									<tbody>
										{#each leafs as l}
											<tr><td>{l.name || l.ip}</td><td class="faint">{l.account}</td><td class="num r">{compact(l.inMsgs + l.outMsgs)} msgs</td></tr>
										{/each}
									</tbody>
								</table>
							</div>
						{/if}
					</div>
				{:else}
					<div class="card-head"><h2>Summary</h2></div>
					<div class="card-body">
						<dl class="kv-list">
							<dt>Servers</dt><dd>{snap.totals.servers}</dd>
							<dt>Clusters</dt><dd>{snap.totals.clusters}</dd>
							<dt>Route links</dt><dd>{snap.routes.length}</dd>
							<dt>Gateway links</dt><dd>{snap.gateways.filter((g) => g.connected).length} / {snap.gateways.length}</dd>
							<dt>Leafnodes</dt><dd>{snap.leafnodes.length}</dd>
						</dl>
						<p class="faint" style="font-size:12.5px">Drag to pan, scroll to zoom. Red dashed lines are routes that should exist inside a cluster but do not.</p>
					</div>
				{/if}
			</div>
		</div>
	{/if}
</div>

<style>
	.layout {
		display: grid;
		grid-template-columns: minmax(0, 1fr) 340px;
		gap: 14px;
		align-items: start;
	}
	@media (max-width: 1100px) {
		.layout {
			grid-template-columns: 1fr;
		}
	}
	.side h3 {
		font-size: 12px;
		color: var(--text-2);
		margin-bottom: 4px;
	}
	.compact td {
		padding: 5px 6px;
		font-size: 12.5px;
	}
	.bad-text {
		color: var(--bad);
	}
</style>
