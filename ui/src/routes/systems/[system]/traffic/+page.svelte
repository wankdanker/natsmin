<script lang="ts">
	import { onMount } from 'svelte';
	import { getLive } from '$lib/context';
	import { findSystem } from '$lib/systems.svelte';
	import { api, unwrap, errMsg, type Schemas } from '$lib/api/client';
	import { bytes, compact, num } from '$lib/format';
	import { Sorter } from '$lib/sort.svelte';
	import SortTh from '$lib/components/SortTh.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import Loading from '$lib/components/Loading.svelte';
	import Empty from '$lib/components/Empty.svelte';
	import NotMonitored from '$lib/components/NotMonitored.svelte';

	type Stat = Schemas['AccountStatOut'];
	const live = getLive();
	const sys = $derived(findSystem(live.id));
	let stats = $state<Stat[] | null>(null);
	let includeUnused = $state(false);
	let error = $state<string | null>(null);
	const sorter = new Sorter<Stat>('conns', -1);

	let subject = $state('');
	let account = $state('');
	let interest = $state<Schemas['InterestOut'] | null>(null);
	let iBusy = $state(false);
	let iError = $state<string | null>(null);

	async function load() {
		try {
			stats = await unwrap(api.GET('/v1/systems/{system}/account-stats', { params: { path: { system: live.id }, query: { includeUnused } } }));
			error = null;
		} catch (e) {
			error = errMsg(e);
		}
	}

	async function lookup(e: SubmitEvent) {
		e.preventDefault();
		iBusy = true;
		iError = null;
		try {
			interest = await unwrap(
				api.GET('/v1/systems/{system}/interest', { params: { path: { system: live.id }, query: { subject, account: account || undefined } } })
			);
		} catch (err) {
			iError = errMsg(err);
		} finally {
			iBusy = false;
		}
	}

	onMount(() => {
		load();
		const t = setInterval(() => !document.hidden && load(), 10000);
		return () => clearInterval(t);
	});
</script>

<svelte:head><title>Accounts & subjects · {sys?.name ?? live.id} · natsmin</title></svelte:head>

<div class="page" style="max-width:none">
	<div class="page-head">
		<div>
			<h1>Accounts & subjects</h1>
			<div class="sub">Traffic per NATS account, and who is listening on a subject.</div>
		</div>
	</div>
	{#if sys && !sys.status.monitored}
		<NotMonitored system={live.id} />
	{:else}
		<div class="card">
			<div class="card-head">
				<h2>Subject interest</h2>
				<span class="faint" style="font-size:12.5px">Find every subscription that would receive a message on a subject.</span>
			</div>
			<form class="card-body row" onsubmit={lookup} style="align-items:flex-end">
				<label class="field" style="flex:2;min-width:220px"><span>Subject</span><input bind:value={subject} placeholder="orders.eu.created" required /></label>
				<label class="field" style="flex:1;min-width:160px"
					><span>Account</span><input bind:value={account} placeholder="Account name or key" list="acc-names" /></label
				>
				<datalist id="acc-names">{#each stats ?? [] as s}<option value={s.account}>{s.name}</option>{/each}</datalist>
				<button class="btn primary" disabled={iBusy}><Icon name="search" />Look up</button>
			</form>
			{#if iError}<div class="card-body"><div class="alert bad">{iError}</div></div>{/if}
			{#if interest}
				{#if interest.total === 0}
					<Empty icon="search" title="Nobody is subscribed">No subscription in {interest.account || 'the default account'} matches <code>{interest.subject}</code>.</Empty>
				{:else}
					<div class="table-wrap">
						<table class="t">
							<thead><tr><th>Server</th><th>Subscription</th><th>Queue group</th><th>Connection</th><th class="r">Delivered</th></tr></thead>
							<tbody>
								{#each interest.servers as s}
									{#each s.subscriptions as sub}
										<tr>
											<td>{s.server}</td>
											<td><code>{sub.subject}</code></td>
											<td>{sub.queue || '—'}</td>
											<td>
												<a href="/systems/{live.id}/connections?server={encodeURIComponent(s.server)}">#{sub.cid}</a>
											</td>
											<td class="r num">{compact(sub.msgs)}</td>
										</tr>
									{/each}
								{/each}
							</tbody>
						</table>
					</div>
				{/if}
			{/if}
		</div>

		<div class="card" style="margin-top:14px">
			<div class="card-head">
				<h2>Accounts</h2>
				<span class="spacer"></span>
				<label class="check"><input type="checkbox" bind:checked={includeUnused} onchange={load} />Include idle accounts</label>
			</div>
			{#if error}<div class="card-body"><div class="alert bad">{error}</div></div>
			{:else if !stats}<Loading />
			{:else if stats.length === 0}<Empty icon="users" title="No active accounts" />
			{:else}
				<div class="table-wrap">
					<table class="t">
						<thead>
							<tr>
								<SortTh {sorter} key="account">Account</SortTh>
								<SortTh {sorter} key="conns" right desc>Connections</SortTh>
								<SortTh {sorter} key="leafnodes" right desc>Leafnodes</SortTh>
								<SortTh {sorter} key="subscriptions" right desc>Subscriptions</SortTh>
								<SortTh {sorter} key="receivedMsgs" right desc>Msgs received</SortTh>
								<SortTh {sorter} key="sentMsgs" right desc>Msgs sent</SortTh>
								<SortTh {sorter} key="receivedBytes" right desc>Bytes received</SortTh>
								<SortTh {sorter} key="sentBytes" right desc>Bytes sent</SortTh>
								<SortTh {sorter} key="slowConsumers" right desc>Slow</SortTh>
								<th>Servers</th>
							</tr>
						</thead>
						<tbody>
							{#each sorter.sort(stats) as s (s.account)}
								<tr>
									<td><b class="mono">{s.name || s.account}</b>{#if s.name && s.name !== s.account}<div class="faint mono" style="font-size:11px">{s.account}</div>{/if}</td>
									<td class="r num"><a href="/systems/{live.id}/connections?account={encodeURIComponent(s.account)}">{num(s.conns)}</a></td>
									<td class="r num">{num(s.leafnodes)}</td>
									<td class="r num">{num(s.subscriptions)}</td>
									<td class="r num">{compact(s.receivedMsgs)}</td>
									<td class="r num">{compact(s.sentMsgs)}</td>
									<td class="r num">{bytes(s.receivedBytes)}</td>
									<td class="r num">{bytes(s.sentBytes)}</td>
									<td class="r num">{s.slowConsumers}</td>
									<td class="faint" style="font-size:12px">{s.servers.length}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{/if}
		</div>
	{/if}
</div>
