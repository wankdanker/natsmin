<script lang="ts">
	import { goto } from '$app/navigation';
	import { getLive } from '$lib/context';
	import { findSystem } from '$lib/systems.svelte';
	import { bytes, compact, num, rate } from '$lib/format';
	import { Sorter } from '$lib/sort.svelte';
	import SortTh from '$lib/components/SortTh.svelte';
	import Meter from '$lib/components/Meter.svelte';
	import Loading from '$lib/components/Loading.svelte';
	import NotMonitored from '$lib/components/NotMonitored.svelte';
	import type { Schemas } from '$lib/api/client';

	type Server = Schemas['ServerNode'];
	const live = getLive();
	const sys = $derived(findSystem(live.id));
	const snap = $derived(live.snapshot);
	let filter = $state('');
	const sorter = new Sorter<Server>('cluster');
	const rows = $derived(
		sorter.sort(
			(snap?.servers ?? []).filter(
				(s) => !filter || `${s.name} ${s.cluster} ${s.host} ${s.version} ${(s.tags ?? []).join(' ')}`.toLowerCase().includes(filter.toLowerCase())
			)
		)
	);
</script>

<svelte:head><title>Servers · {sys?.name ?? live.id} · natsmin</title></svelte:head>

<div class="page" style="max-width:none">
	<div class="page-head">
		<div>
			<h1>Servers</h1>
			<div class="sub">{snap?.servers.length ?? 0} servers across {snap?.totals.clusters ?? 0} clusters</div>
		</div>
		<span class="spacer"></span>
		<input type="search" placeholder="Filter by name, cluster, tag…" bind:value={filter} style="width:260px" />
	</div>
	{#if sys && !sys.status.monitored}
		<NotMonitored system={live.id} />
	{:else if !snap}
		<Loading />
	{:else}
		<div class="card table-wrap">
			<table class="t">
				<thead>
					<tr>
						<SortTh {sorter} key="name">Server</SortTh>
						<SortTh {sorter} key="cluster">Cluster</SortTh>
						<SortTh {sorter} key="version">Version</SortTh>
						<SortTh {sorter} key="healthy">Health</SortTh>
						<SortTh {sorter} key="cpu" right desc>CPU</SortTh>
						<SortTh {sorter} key="mem" right desc>Memory</SortTh>
						<SortTh {sorter} key="connections" right desc>Conns</SortTh>
						<SortTh {sorter} key="subscriptions" right desc>Subs</SortTh>
						<SortTh {sorter} key="inMsgsRate" right desc>In/s</SortTh>
						<SortTh {sorter} key="outMsgsRate" right desc>Out/s</SortTh>
						<SortTh {sorter} key="slowConsumers" right desc>Slow</SortTh>
						<th>JetStream storage</th>
						<SortTh {sorter} key="uptime">Uptime</SortTh>
					</tr>
				</thead>
				<tbody>
					{#each rows as s (s.id)}
						<tr class="clickable" onclick={() => goto(`/systems/${live.id}/servers/${s.id}`)}>
							<td>
								<div class="row" style="gap:6px;flex-wrap:nowrap">
									<span class="dot {s.missing ? '' : s.healthy ? 'ok' : 'bad'}"></span>
									<b>{s.name}</b>
									{#if s.jetstream?.metaLeader}<span class="badge accent" title="JetStream meta leader">leader</span>{/if}
								</div>
								<div class="faint mono" style="font-size:11.5px">{s.host}:{s.port}</div>
							</td>
							<td>{s.cluster || '—'}</td>
							<td class="mono">{s.version}</td>
							<td>
								{#if s.missing}<span class="badge">not answering</span>
								{:else if s.healthy}<span class="badge ok">ok</span>
								{:else}<span class="badge bad" title={s.healthError}>unhealthy</span>{/if}
							</td>
							<td class="r num">{s.cpu.toFixed(1)}%</td>
							<td class="r num">{bytes(s.mem)}</td>
							<td class="r num">{num(s.connections)}</td>
							<td class="r num">{compact(s.subscriptions)}</td>
							<td class="r num">{rate(s.inMsgsRate)}</td>
							<td class="r num">{rate(s.outMsgsRate)}</td>
							<td class="r num">{#if s.slowConsumers > 0}<span class="badge warn">{num(s.slowConsumers)}</span>{:else}0{/if}</td>
							<td style="min-width:140px">
								{#if s.jetstream}
									<div class="faint" style="font-size:11.5px">{bytes(s.jetstream.storage)} / {bytes(s.jetstream.maxStorage)}</div>
									<Meter used={s.jetstream.storage} total={s.jetstream.maxStorage} />
								{:else}<span class="faint">off</span>{/if}
							</td>
							<td class="nowrap">{s.uptime}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>
