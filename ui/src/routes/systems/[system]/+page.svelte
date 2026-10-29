<script lang="ts">
	import { getLive } from '$lib/context';
	import { findSystem } from '$lib/systems.svelte';
	import { bytes, bytesRate, compact, num, rate, ago, nanos } from '$lib/format';
	import { errMsg } from '$lib/api/client';
	import { toast } from '$lib/toast.svelte';
	import Stat from '$lib/components/Stat.svelte';
	import Chart from '$lib/components/Chart.svelte';
	import Meter from '$lib/components/Meter.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import Loading from '$lib/components/Loading.svelte';
	import NotMonitored from '$lib/components/NotMonitored.svelte';
	import Topology from '$lib/components/Topology.svelte';

	const live = getLive();
	const sys = $derived(findSystem(live.id));
	const snap = $derived(live.snapshot);
	const h = $derived(live.history);
	const times = $derived(h.map((p) => new Date(p.t).getTime()));
	const issues = $derived(
		(snap?.servers ?? []).filter((s) => s.missing || !s.healthy || s.slowConsumers > 0 || s.cpu > 85)
	);
	let refreshing = $state(false);

	async function refresh() {
		refreshing = true;
		try {
			await live.refresh();
		} catch (e) {
			toast(errMsg(e), 'bad');
		} finally {
			refreshing = false;
		}
	}
</script>

<svelte:head><title>{sys?.name ?? live.id} · natsmin</title></svelte:head>

<div class="page">
	<div class="page-head">
		<div>
			<h1>{sys?.name ?? live.id}</h1>
			<div class="sub">
				{#if sys?.description}{sys.description} · {/if}
				{#if snap}updated {ago(snap.time)}{/if}
				{#if live.connected}<span class="badge ok" style="margin-left:6px"><span class="dot ok"></span>live</span>{/if}
			</div>
		</div>
		<span class="spacer"></span>
		{#if sys?.status.monitored}
			<button class="btn" onclick={refresh} disabled={refreshing}><Icon name="refresh" />Refresh</button>
		{/if}
	</div>

	{#if sys && !sys.status.monitored}
		<NotMonitored system={live.id} />
		{#if sys.accounts.length}
			<div class="card" style="margin-top:14px">
				<div class="card-head"><h2>Accounts</h2></div>
				<div class="card-body row">
					{#each sys.accounts as a}
						<a class="btn" href="/systems/{live.id}/a/{a.id}/streams"><Icon name="stream" />{a.name}</a>
					{/each}
				</div>
			</div>
		{/if}
	{:else if !snap}
		<Loading text="Polling the system…" />
	{:else}
		{#if snap.error}
			<div class="alert bad" style="margin-bottom:14px"><b>Cannot reach the system:</b> {snap.error}</div>
		{/if}

		<div class="kpis">
			<Stat
				label="Servers healthy"
				value="{snap.servers.filter((s) => s.healthy && !s.missing).length} / {snap.servers.length}"
				tone={snap.servers.some((s) => !s.healthy || s.missing) ? 'bad' : 'ok'}
				sub="{snap.totals.clusters} cluster{snap.totals.clusters === 1 ? '' : 's'} · v{snap.totals.versions.join(', v')}"
			/>
			<Stat label="Connections" value={num(snap.totals.connections)} spark={h.map((p) => p.connections)} />
			<Stat label="Subscriptions" value={compact(snap.totals.subscriptions)} spark={h.map((p) => p.subscriptions)} />
			<Stat label="Messages in" value={rate(snap.totals.inMsgsRate)} spark={h.map((p) => p.inMsgs)} sub={bytesRate(snap.totals.inBytesRate)} />
			<Stat label="Messages out" value={rate(snap.totals.outMsgsRate)} spark={h.map((p) => p.outMsgs)} sub={bytesRate(snap.totals.outBytesRate)} />
			<Stat
				label="Slow consumers"
				value={num(snap.totals.slowConsumers)}
				tone={snap.totals.slowConsumers > 0 ? 'warn' : undefined}
				sub="since server start"
			/>
			<Stat label="Leafnodes" value={num(snap.totals.leafnodes)} sub="{snap.gateways.filter((g) => g.connected).length} gateway links" />
		</div>

		<div class="grid-2" style="margin-top:14px">
			<div class="card">
				<div class="card-head"><h2>Message rate</h2></div>
				<div class="card-body">
					<Chart
						{times}
						format={(n) => compact(n)}
						series={[
							{ name: 'in /s', values: h.map((p) => p.inMsgs), color: 'var(--series-1)' },
							{ name: 'out /s', values: h.map((p) => p.outMsgs), color: 'var(--series-2)' }
						]}
					/>
				</div>
			</div>
			<div class="card">
				<div class="card-head"><h2>Throughput</h2></div>
				<div class="card-body">
					<Chart
						{times}
						format={(n) => bytes(n, 0)}
						series={[
							{ name: 'in /s', values: h.map((p) => p.inBytes), color: 'var(--series-4)' },
							{ name: 'out /s', values: h.map((p) => p.outBytes), color: 'var(--series-3)' }
						]}
					/>
				</div>
			</div>
		</div>

		<div class="split" style="margin-top:14px">
			<div class="card">
				<div class="card-head">
					<h2>Topology</h2>
					<span class="spacer"></span>
					<a class="btn sm" href="/systems/{live.id}/topology">Open<Icon name="chevronRight" size={14} /></a>
				</div>
				<div class="card-body"><Topology snapshot={snap} height={380} /></div>
			</div>

			<div class="stack">
				<div class="card">
					<div class="card-head">
						<h2>Attention</h2>
						<span class="spacer"></span>
						{#if issues.length === 0}<span class="badge ok">All clear</span>{:else}<span class="badge warn">{issues.length}</span>{/if}
					</div>
					{#if issues.length === 0}
						<div class="card-body faint">Every server is healthy with no slow consumers.</div>
					{:else}
						<table class="t">
							<tbody>
								{#each issues as s}
									<tr>
										<td><a href="/systems/{live.id}/servers/{s.id}">{s.name}</a></td>
										<td>
											{#if s.missing}<span class="badge bad">not answering</span>
											{:else if !s.healthy}<span class="badge bad" title={s.healthError}>unhealthy</span>{/if}
											{#if s.slowConsumers > 0}<span class="badge warn">{s.slowConsumers} slow consumers</span>{/if}
											{#if s.cpu > 85}<span class="badge warn">CPU {s.cpu.toFixed(0)}%</span>{/if}
										</td>
									</tr>
									{#if s.healthError}<tr><td colspan="2" class="faint" style="font-size:12px">{s.healthError}</td></tr>{/if}
								{/each}
							</tbody>
						</table>
					{/if}
				</div>

				{#if snap.jetstream}
					{@const js = snap.jetstream}
					<div class="card">
						<div class="card-head">
							<h2>JetStream</h2>
							<span class="spacer"></span>
							<a class="btn sm" href="/systems/{live.id}/jetstream">Details<Icon name="chevronRight" size={14} /></a>
						</div>
						<div class="card-body stack">
							<dl class="kv-list">
								<dt>Meta leader</dt>
								<dd><b>{js.metaLeader || '—'}</b> · {js.metaSize} peers{js.metaPending ? ` · ${js.metaPending} pending` : ''}</dd>
								<dt>Streams</dt>
								<dd class="num">{num(js.streams)}</dd>
								<dt>Consumers</dt>
								<dd class="num">{num(js.consumers)}</dd>
								<dt>Messages</dt>
								<dd class="num">{compact(js.messages)} <span class="faint">incl. replicas</span></dd>
								<dt>API errors</dt>
								<dd class="num">{num(js.apiErrors)} / {compact(js.apiTotal)}</dd>
							</dl>
							<div>
								<div class="row" style="font-size:12px"><span class="muted">File storage</span><span class="spacer"></span>{bytes(js.storage)} / {bytes(js.maxStorage)}</div>
								<Meter used={js.storage} total={js.maxStorage} />
							</div>
							<div>
								<div class="row" style="font-size:12px"><span class="muted">Memory storage</span><span class="spacer"></span>{bytes(js.memory)} / {bytes(js.maxMemory)}</div>
								<Meter used={js.memory} total={js.maxMemory} />
							</div>
							{#if js.peers.some((p) => !p.current || p.offline)}
								<div class="alert warn">
									Lagging meta peers:
									{js.peers
										.filter((p) => !p.current || p.offline)
										.map((p) => `${p.name}${p.offline ? ' (offline)' : ` (lag ${p.lag}, seen ${nanos(p.activeNanos)} ago)`}`)
										.join(', ')}
								</div>
							{/if}
						</div>
					</div>
				{/if}
			</div>
		</div>

		<div class="card" style="margin-top:14px">
			<div class="card-head"><h2>Clusters</h2></div>
			<div class="table-wrap">
				<table class="t">
					<thead><tr><th>Cluster</th><th>Servers</th><th>Healthy</th><th class="r">Connections</th><th>Gateways</th></tr></thead>
					<tbody>
						{#each snap.clusters as c}
							{@const out = snap.gateways.filter((g) => g.fromCluster === c.name)}
							<tr>
								<td><b>{c.name}</b></td>
								<td>{c.servers.length}</td>
								<td><span class="badge {c.healthy === c.servers.length ? 'ok' : 'bad'}">{c.healthy}/{c.servers.length}</span></td>
								<td class="r num">{num(c.connections)}</td>
								<td>
									{#each [...new Set(out.map((g) => g.toCluster))] as to}
										{@const links = out.filter((g) => g.toCluster === to)}
										<span class="badge {links.every((g) => g.connected) ? '' : 'warn'}">→ {to} {links.filter((g) => g.connected).length}/{links.length}</span>
									{/each}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		</div>
	{/if}
</div>

<style>
	.split {
		display: grid;
		grid-template-columns: minmax(0, 1.6fr) minmax(300px, 1fr);
		gap: 14px;
		align-items: start;
	}
	@media (max-width: 1100px) {
		.split {
			grid-template-columns: 1fr;
		}
	}
</style>
