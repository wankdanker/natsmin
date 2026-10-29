<script lang="ts">
	import { onMount } from 'svelte';
	import { getLive } from '$lib/context';
	import { findSystem } from '$lib/systems.svelte';
	import { api, unwrap, errMsg, type Schemas } from '$lib/api/client';
	import { can } from '$lib/session.svelte';
	import { toast } from '$lib/toast.svelte';
	import { confirm } from '$lib/components/Confirm.svelte';
	import { bytes, compact, nanos, num, ago } from '$lib/format';
	import { Sorter } from '$lib/sort.svelte';
	import SortTh from '$lib/components/SortTh.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import Loading from '$lib/components/Loading.svelte';
	import Empty from '$lib/components/Empty.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import Meter from '$lib/components/Meter.svelte';
	import NotMonitored from '$lib/components/NotMonitored.svelte';

	type Placement = Schemas['StreamPlacement'];
	const live = getLive();
	const sys = $derived(findSystem(live.id));
	const snap = $derived(live.snapshot);
	const js = $derived(snap?.jetstream);
	let assets = $state<Placement[] | null>(null);
	let error = $state<string | null>(null);
	let filter = $state('');
	let onlyDegraded = $state(false);
	const sorter = new Sorter<Placement>('accountName');

	const rows = $derived(
		sorter.sort(
			(assets ?? []).filter(
				(a) =>
					(!onlyDegraded || a.degraded) &&
					(!filter || `${a.stream} ${a.accountName} ${a.cluster} ${a.replicas.map((r) => r.server).join(' ')}`.toLowerCase().includes(filter.toLowerCase()))
			)
		)
	);
	const clusters = $derived(snap?.clusters.map((c) => c.name) ?? []);
	const jsServers = $derived(snap?.servers.filter((s) => s.jetstream) ?? []);

	async function load() {
		try {
			assets = await unwrap(api.GET('/v1/systems/{system}/jetstream/assets', { params: { path: { system: live.id } } }));
			error = null;
		} catch (e) {
			error = errMsg(e);
		}
	}
	onMount(() => {
		load();
		const t = setInterval(() => !document.hidden && load(), 10000);
		return () => clearInterval(t);
	});

	async function stepdownMeta(preferred?: string) {
		if (!(await confirm({ title: 'Step down meta leader', message: preferred ? `Hand JetStream meta leadership to ${preferred}?` : 'Ask the JetStream meta leader to hand over to another peer?', confirmText: 'Step down' })))
			return;
		try {
			await unwrap(api.POST('/v1/systems/{system}/jetstream/meta/stepdown', { params: { path: { system: live.id } }, body: { preferred } }));
			toast('Meta leader stepped down');
			setTimeout(() => live.refresh(), 1500);
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}

	async function removePeer(name: string, peer: string) {
		if (
			!(await confirm({
				title: 'Remove peer',
				message: `Remove ${name} from the JetStream meta group?\n\nOnly do this when the server is gone for good. Its replicas are re-placed on other servers.`,
				confirmText: 'Remove peer',
				danger: true,
				typeToConfirm: name
			}))
		)
			return;
		try {
			await unwrap(api.POST('/v1/systems/{system}/jetstream/peers/remove', { params: { path: { system: live.id } }, body: { server: name, peerId: peer } }));
			toast(`${name} removed from the meta group`);
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}

	// Move dialog.
	let moveOpen = $state(false);
	let moveTarget = $state<Placement | null>(null);
	let moveServer = $state('');
	let moveCluster = $state('');
	let moveTags = $state('');
	let moving = $state(false);

	function openMove(a: Placement) {
		moveTarget = a;
		moveServer = '';
		moveCluster = '';
		moveTags = '';
		moveOpen = true;
	}

	async function doMove(cancel = false) {
		if (!moveTarget) return;
		moving = true;
		try {
			await unwrap(
				api.POST('/v1/systems/{system}/jetstream/streams/move', {
					params: { path: { system: live.id } },
					body: {
						account: moveTarget.account,
						stream: moveTarget.stream,
						server: moveServer || undefined,
						cluster: moveCluster || undefined,
						tags: moveTags ? moveTags.split(',').map((t) => t.trim()) : undefined,
						cancel
					}
				})
			);
			toast(cancel ? 'Move cancelled' : `Moving ${moveTarget.stream}…`);
			moveOpen = false;
			setTimeout(load, 1500);
		} catch (e) {
			toast(errMsg(e), 'bad');
		} finally {
			moving = false;
		}
	}

	// Drain: move every stream with a replica on a server.
	async function drain(server: string) {
		const affected = (assets ?? []).filter((a) => a.replicas.some((r) => r.server === server));
		if (affected.length === 0) {
			toast(`No streams have replicas on ${server}`, 'info');
			return;
		}
		if (
			!(await confirm({
				title: `Evacuate ${server}`,
				message: `Move ${affected.length} stream replica${affected.length === 1 ? '' : 's'} off ${server}? Do this before taking the server down for maintenance.`,
				confirmText: 'Evacuate',
				danger: true
			}))
		)
			return;
		let ok = 0;
		for (const a of affected) {
			try {
				await unwrap(api.POST('/v1/systems/{system}/jetstream/streams/move', { params: { path: { system: live.id } }, body: { account: a.account, stream: a.stream, server } }));
				ok++;
			} catch (e) {
				toast(`${a.stream}: ${errMsg(e)}`, 'bad');
			}
		}
		toast(`Started moving ${ok} of ${affected.length} streams off ${server}`);
		setTimeout(load, 2000);
	}
</script>

<svelte:head><title>JetStream · {sys?.name ?? live.id} · natsmin</title></svelte:head>

<div class="page" style="max-width:none">
	<div class="page-head">
		<div>
			<h1>JetStream</h1>
			<div class="sub">The meta group and where every stream's replicas live, across all accounts.</div>
		</div>
	</div>

	{#if sys && !sys.status.monitored}
		<NotMonitored system={live.id} />
	{:else if !snap}
		<Loading />
	{:else if !js}
		<div class="card"><Empty icon="db" title="JetStream is not enabled">None of the servers in this system run JetStream.</Empty></div>
	{:else}
		<div class="grid-2">
			<div class="card">
				<div class="card-head">
					<h2>Meta group</h2>
					<span class="badge">{js.peers.length} peers</span>
					<span class="spacer"></span>
					{#if can('admin')}<button class="btn sm" onclick={() => stepdownMeta()}><Icon name="swap" size={14} />Step down leader</button>{/if}
				</div>
				<div class="table-wrap">
					<table class="t">
						<thead><tr><th>Peer</th><th>State</th><th class="r">Lag</th><th>Last seen</th>{#if can('admin')}<th></th>{/if}</tr></thead>
						<tbody>
							{#each js.peers as p}
								<tr>
									<td>{#if p.leader}<span title="leader">★</span> {/if}<b>{p.name}</b><div class="faint mono" style="font-size:11px">{p.peer}</div></td>
									<td>
										{#if p.leader}<span class="badge accent">leader</span>
										{:else if p.offline}<span class="badge bad">offline</span>
										{:else if p.current}<span class="badge ok">current</span>
										{:else}<span class="badge warn">catching up</span>{/if}
									</td>
									<td class="r num">{num(p.lag)}</td>
									<td>{p.leader ? '—' : nanos(p.activeNanos) + ' ago'}</td>
									{#if can('admin')}
										<td class="nowrap">
											{#if !p.leader}<button class="btn sm" onclick={() => stepdownMeta(p.name)} title="Make leader"><Icon name="crown" size={13} /></button>{/if}
											{#if p.offline}<button class="btn sm danger" onclick={() => removePeer(p.name, p.peer)}>Remove</button>{/if}
										</td>
									{/if}
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			</div>

			<div class="card">
				<div class="card-head"><h2>Storage by server</h2></div>
				<div class="table-wrap">
					<table class="t">
						<thead><tr><th>Server</th><th>File</th><th>Memory</th><th class="r">Streams led</th>{#if can('admin')}<th></th>{/if}</tr></thead>
						<tbody>
							{#each jsServers as s}
								{@const j = s.jetstream!}
								<tr>
									<td><b>{s.name}</b><div class="faint" style="font-size:11.5px">{s.cluster}</div></td>
									<td style="min-width:140px"><div class="faint" style="font-size:11.5px">{bytes(j.storage)} / {bytes(j.maxStorage)}</div><Meter used={j.storage} total={j.maxStorage} /></td>
									<td style="min-width:120px"><div class="faint" style="font-size:11.5px">{bytes(j.memory)} / {bytes(j.maxMemory)}</div><Meter used={j.memory} total={j.maxMemory} /></td>
									<td class="r num">{j.streamsLeader} / {j.streams}</td>
									{#if can('admin')}<td><button class="btn sm" onclick={() => drain(s.name)} title="Move every stream replica off this server">Evacuate</button></td>{/if}
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			</div>
		</div>

		<div class="card" style="margin-top:14px">
			<div class="card-head">
				<h2>Streams in all accounts</h2>
				{#if assets}<span class="badge">{assets.length}</span>{/if}
				{#if assets?.some((a) => a.degraded)}<span class="badge warn">{assets.filter((a) => a.degraded).length} degraded</span>{/if}
				<span class="spacer"></span>
				<label class="check"><input type="checkbox" bind:checked={onlyDegraded} />Degraded only</label>
				<input type="search" placeholder="Filter…" bind:value={filter} style="width:200px" />
			</div>
			{#if error}<div class="card-body"><div class="alert bad">{error}</div></div>
			{:else if !assets}<Loading />
			{:else if rows.length === 0}<Empty icon="stream" title="No streams" />
			{:else}
				<div class="table-wrap">
					<table class="t">
						<thead>
							<tr>
								<SortTh {sorter} key="accountName">Account</SortTh>
								<SortTh {sorter} key="stream">Stream</SortTh>
								<SortTh {sorter} key="cluster">Cluster</SortTh>
								<th>Replicas</th>
								<SortTh {sorter} key="messages" right desc>Messages</SortTh>
								<SortTh {sorter} key="bytes" right desc>Size</SortTh>
								<SortTh {sorter} key="consumers" right desc>Consumers</SortTh>
								<SortTh {sorter} key="created">Created</SortTh>
								{#if can('admin')}<th></th>{/if}
							</tr>
						</thead>
						<tbody>
							{#each rows as a (a.account + a.stream)}
								<tr>
									<td class="mono">{a.accountName || a.account}</td>
									<td>
										<b>{a.stream}</b>
										{#if a.degraded}<span class="badge warn">degraded</span>{/if}
										{#if a.mirror}<span class="badge info">mirror of {a.mirror}</span>{/if}
										{#if a.sources?.length}<span class="badge info">{a.sources.length} sources</span>{/if}
										<div class="faint mono" style="font-size:11px">{(a.subjects ?? []).join(', ')}</div>
									</td>
									<td>{a.cluster || '—'}</td>
									<td>
										<div class="reps">
											{#each a.replicas as r}
												<span
													class="rep"
													class:leader={r.leader}
													class:bad={r.offline}
													class:warn={!r.current && !r.offline}
													title="{r.server}{r.leader ? ' (leader)' : ''}{r.offline ? ' offline' : !r.current ? ` lag ${r.lag}` : ''}"
													>{r.leader ? '★ ' : ''}{r.server}</span
												>
											{/each}
										</div>
									</td>
									<td class="r num">{compact(a.messages)}</td>
									<td class="r num">{bytes(a.bytes)}</td>
									<td class="r num">{a.consumers}</td>
									<td class="nowrap">{ago(a.created)}</td>
									{#if can('admin')}<td><button class="btn sm" onclick={() => openMove(a)}>Move</button></td>{/if}
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{/if}
		</div>
	{/if}
</div>

<Modal bind:open={moveOpen} title="Move {moveTarget?.stream ?? ''}">
	<div class="stack">
		<p class="muted" style="margin:0">
			Moves the stream's replicas. Pick a server to move off, a target cluster, placement tags, or a combination.
			Large streams take time to copy; the old replicas are removed once the new ones catch up.
		</p>
		<label class="field">
			<span>Move off server</span>
			<select bind:value={moveServer}>
				<option value="">(any)</option>
				{#each moveTarget?.replicas ?? [] as r}<option value={r.server}>{r.server}</option>{/each}
			</select>
		</label>
		<label class="field">
			<span>Target cluster</span>
			<select bind:value={moveCluster}>
				<option value="">(same cluster)</option>
				{#each clusters as c}<option value={c}>{c}</option>{/each}
			</select>
		</label>
		<label class="field"><span>Placement tags</span><input bind:value={moveTags} placeholder="az:2, ssd" /></label>
	</div>
	{#snippet footer()}
		<button class="btn" onclick={() => doMove(true)} disabled={moving}>Cancel a running move</button>
		<span class="spacer"></span>
		<button class="btn" onclick={() => (moveOpen = false)}>Close</button>
		<button class="btn primary" onclick={() => doMove()} disabled={moving || (!moveServer && !moveCluster && !moveTags)}>Move</button>
	{/snippet}
</Modal>

<style>
	.reps {
		display: flex;
		flex-wrap: wrap;
		gap: 4px;
	}
	.rep {
		font-size: 11.5px;
		padding: 1px 7px;
		border-radius: 4px;
		border: 1px solid var(--border);
		background: var(--bg-sunken);
		white-space: nowrap;
	}
	.rep.leader {
		border-color: var(--accent);
		color: var(--accent);
	}
	.rep.warn {
		border-color: var(--warn);
		color: var(--warn);
	}
	.rep.bad {
		border-color: var(--bad);
		color: var(--bad);
		text-decoration: line-through;
	}
</style>
