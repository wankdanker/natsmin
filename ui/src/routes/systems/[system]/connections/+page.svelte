<script lang="ts">
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { getLive } from '$lib/context';
	import { findSystem } from '$lib/systems.svelte';
	import { api, unwrap, errMsg, type Schemas } from '$lib/api/client';
	import { can } from '$lib/session.svelte';
	import { toast } from '$lib/toast.svelte';
	import { confirm } from '$lib/components/Confirm.svelte';
	import { bytes, compact, num, dateTime } from '$lib/format';
	import Icon from '$lib/components/Icon.svelte';
	import Loading from '$lib/components/Loading.svelte';
	import Empty from '$lib/components/Empty.svelte';
	import NotMonitored from '$lib/components/NotMonitored.svelte';

	type Conn = Schemas['ConnOut'];
	type Sort = 'cid' | 'start' | 'subs' | 'pending' | 'msgs_to' | 'msgs_from' | 'bytes_to' | 'bytes_from' | 'last' | 'uptime' | 'stop';
	const live = getLive();
	const sys = $derived(findSystem(live.id));

	let server = $state(page.url.searchParams.get('server') ?? '');
	let account = $state(page.url.searchParams.get('account') ?? '');
	let user = $state('');
	let subject = $state('');
	let stateF = $state<'open' | 'closed' | 'all'>('open');
	let sort = $state<Sort>('msgs_from');
	let limit = $state(100);
	let offset = $state(0);
	let auto = $state(true);
	let data = $state<Schemas['ConnectionsOut'] | null>(null);
	let loading = $state(false);
	let error = $state<string | null>(null);
	let expanded = $state<string | null>(null);

	async function load() {
		loading = true;
		try {
			data = await unwrap(
				api.GET('/v1/systems/{system}/connections', {
					params: {
						path: { system: live.id },
						query: {
							server: server || undefined,
							account: account || undefined,
							user: user || undefined,
							subject: subject || undefined,
							state: stateF,
							sort,
							limit,
							offset,
							subs: true
						}
					}
				})
			);
			error = null;
		} catch (e) {
			error = errMsg(e);
		} finally {
			loading = false;
		}
	}

	onMount(() => {
		load();
		const t = setInterval(() => auto && !document.hidden && load(), 5000);
		return () => clearInterval(t);
	});

	function apply(e?: SubmitEvent) {
		e?.preventDefault();
		offset = 0;
		load();
	}

	async function kick(c: Conn, ldm = false) {
		const what = ldm ? 'Send a lame duck notice to' : 'Disconnect';
		if (!(await confirm({ title: ldm ? 'Ask client to move' : 'Disconnect client', message: `${what} connection ${c.cid} (${c.name || c.ip}) on ${c.server}?`, confirmText: ldm ? 'Send notice' : 'Disconnect', danger: !ldm })))
			return;
		try {
			const path = { system: live.id, server: c.serverId, cid: c.cid };
			if (ldm) await unwrap(api.POST('/v1/systems/{system}/servers/{server}/connections/{cid}/lame-duck', { params: { path } }));
			else await unwrap(api.POST('/v1/systems/{system}/servers/{server}/connections/{cid}/kick', { params: { path } }));
			toast(ldm ? 'Lame duck notice sent' : 'Client disconnected');
			load();
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}

	const sortOptions: [Sort, string][] = [
		['msgs_from', 'Msgs from client'],
		['msgs_to', 'Msgs to client'],
		['bytes_from', 'Bytes from client'],
		['bytes_to', 'Bytes to client'],
		['subs', 'Subscriptions'],
		['pending', 'Pending bytes'],
		['last', 'Last activity'],
		['start', 'Oldest first'],
		['cid', 'Connection id'],
		['stop', 'Recently closed']
	];
</script>

<svelte:head><title>Connections · {sys?.name ?? live.id} · natsmin</title></svelte:head>

<div class="page" style="max-width:none">
	<div class="page-head">
		<div>
			<h1>Connections</h1>
			<div class="sub">{#if data}{num(data.total)} matching across {data.servers} servers{/if}</div>
		</div>
		<span class="spacer"></span>
		<label class="check"><input type="checkbox" bind:checked={auto} />Auto refresh</label>
		<button class="btn" onclick={() => load()} disabled={loading}><Icon name="refresh" />Refresh</button>
	</div>

	{#if sys && !sys.status.monitored}
		<NotMonitored system={live.id} />
	{:else}
		<form class="card filters" onsubmit={apply}>
			<label class="field"><span>Server</span><input bind:value={server} placeholder="name contains…" /></label>
			<label class="field"><span>Account</span><input bind:value={account} placeholder="APP or A…" /></label>
			<label class="field"><span>User</span><input bind:value={user} /></label>
			<label class="field"><span>Subscribed to</span><input bind:value={subject} placeholder="orders.>" /></label>
			<label class="field"
				><span>State</span><select bind:value={stateF} onchange={() => apply()}>
					<option value="open">Open</option><option value="closed">Closed</option><option value="all">All</option>
				</select></label
			>
			<label class="field"
				><span>Sort by</span><select bind:value={sort} onchange={() => apply()}>
					{#each sortOptions as [v, l]}<option value={v}>{l}</option>{/each}
				</select></label
			>
			<div class="field"><span>&nbsp;</span><button class="btn primary"><Icon name="filter" />Apply</button></div>
		</form>

		{#if error}<div class="alert bad" style="margin-top:14px">{error}</div>{/if}

		<div class="card table-wrap" style="margin-top:14px">
			{#if !data}
				<Loading />
			{:else if data.connections.length === 0}
				<Empty icon="plug" title="No connections match" />
			{:else}
				<table class="t">
					<thead>
						<tr>
							<th>Client</th><th>Server</th><th>Account / user</th><th class="r">Subs</th><th class="r">Msgs from</th><th class="r">Msgs to</th
							><th class="r">Bytes from</th><th class="r">Bytes to</th><th class="r">Pending</th><th>RTT</th><th>Uptime</th><th>Idle</th>
							{#if can('operator')}<th></th>{/if}
						</tr>
					</thead>
					<tbody>
						{#each data.connections as c (c.serverId + c.cid)}
							{@const key = c.serverId + c.cid}
							<tr class="clickable" onclick={() => (expanded = expanded === key ? null : key)}>
								<td>
									<b>{c.name || '(unnamed)'}</b>
									<div class="faint" style="font-size:11.5px">#{c.cid} · {c.ip}:{c.port} · {c.lang} {c.version}</div>
								</td>
								<td>{c.server}</td>
								<td><span class="mono">{c.account}</span>{#if c.authorizedUser}<div class="faint">{c.authorizedUser}</div>{/if}</td>
								<td class="r num">{num(c.subscriptions)}</td>
								<td class="r num">{compact(c.inMsgs)}</td>
								<td class="r num">{compact(c.outMsgs)}</td>
								<td class="r num">{bytes(c.inBytes)}</td>
								<td class="r num">{bytes(c.outBytes)}</td>
								<td class="r num">{#if c.pendingBytes > 0}<span class="badge warn">{bytes(c.pendingBytes)}</span>{:else}0{/if}</td>
								<td class="num">{c.rtt ?? ''}</td>
								<td class="nowrap">{c.uptime}</td>
								<td class="nowrap">{#if c.stop}<span class="badge">{c.reason || 'closed'}</span>{:else}{c.idle}{/if}</td>
								{#if can('operator')}
									<td class="nowrap" onclick={(e) => e.stopPropagation()}>
										{#if !c.stop}
											<button class="btn sm" title="Ask client to reconnect elsewhere" onclick={() => kick(c, true)}><Icon name="swap" size={13} /></button>
											<button class="btn sm danger" title="Disconnect" onclick={() => kick(c)}><Icon name="x" size={13} /></button>
										{/if}
									</td>
								{/if}
							</tr>
							{#if expanded === key}
								<tr class="detail">
									<td colspan="13">
										<div class="grid-2">
											<dl class="kv-list">
												<dt>Connected</dt><dd>{dateTime(c.start)}</dd>
												<dt>Last activity</dt><dd>{dateTime(c.lastActivity)}</dd>
												{#if c.stop}<dt>Closed</dt><dd>{dateTime(c.stop)} · {c.reason}</dd>{/if}
												<dt>TLS</dt><dd>{c.tlsVersion || 'no'}</dd>
												{#if c.nameTag}<dt>Name tag</dt><dd>{c.nameTag}</dd>{/if}
												{#if c.tags?.length}<dt>Tags</dt><dd>{c.tags.join(', ')}</dd>{/if}
											</dl>
											<div>
												<div class="label" style="margin-bottom:4px">Subscriptions ({c.subscriptionsList?.length ?? 0})</div>
												<div class="subs">
													{#each c.subscriptionsList ?? [] as s}<code>{s}</code>{:else}<span class="faint">none</span>{/each}
												</div>
											</div>
										</div>
									</td>
								</tr>
							{/if}
						{/each}
					</tbody>
				</table>
			{/if}
		</div>
		{#if data && data.total > limit}
			<div class="row" style="margin-top:10px">
				<button class="btn sm" disabled={offset === 0} onclick={() => ((offset = Math.max(0, offset - limit)), load())}>Previous</button>
				<span class="faint">{offset + 1}–{Math.min(offset + limit, data.total)} of {num(data.total)}</span>
				<button class="btn sm" disabled={offset + limit >= data.total} onclick={() => ((offset += limit), load())}>Next</button>
			</div>
		{/if}
	{/if}
</div>

<style>
	.filters {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
		gap: 10px;
		padding: 12px 14px;
	}
	.detail td {
		background: var(--bg-sunken) !important;
	}
	.subs {
		display: flex;
		flex-wrap: wrap;
		gap: 4px;
		max-height: 160px;
		overflow: auto;
	}
	.subs code {
		background: var(--bg-elev);
		border: 1px solid var(--border);
		padding: 1px 6px;
		border-radius: 4px;
	}
</style>
