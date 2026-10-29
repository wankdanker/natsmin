<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api, unwrap, errMsg } from '$lib/api/client';
	import { can } from '$lib/session.svelte';
	import { toast } from '$lib/toast.svelte';
	import { getLive } from '$lib/context';
	import type { StreamInfo, AccountInfo, StreamConfig } from '$lib/nats';
	import { bytes, compact, ago, num } from '$lib/format';
	import { Sorter } from '$lib/sort.svelte';
	import AccountHead from '$lib/AccountHead.svelte';
	import SortTh from '$lib/components/SortTh.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import Loading from '$lib/components/Loading.svelte';
	import Empty from '$lib/components/Empty.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import Meter from '$lib/components/Meter.svelte';
	import StreamForm from '$lib/components/StreamForm.svelte';

	const path = $derived({ system: page.params.system!, account: page.params.account! });
	const live = getLive();
	let streams = $state<StreamInfo[] | null>(null);
	let info = $state<AccountInfo | null>(null);
	let error = $state<string | null>(null);
	let filter = $state('');
	let createOpen = $state(false);
	let creating = $state(false);
	const sorter = new Sorter<StreamInfo>('config.name');
	const rows = $derived(
		sorter.sort(
			(streams ?? []).filter((s) => !filter || `${s.config.name} ${(s.config.subjects ?? []).join(' ')} ${s.config.description ?? ''}`.toLowerCase().includes(filter.toLowerCase()))
		)
	);
	const clusters = $derived(live.snapshot?.clusters.map((c) => c.name) ?? []);

	async function load() {
		try {
			const [s, i] = await Promise.all([
				unwrap(api.GET('/v1/systems/{system}/accounts/{account}/jetstream/streams', { params: { path } })),
				unwrap(api.GET('/v1/systems/{system}/accounts/{account}/jetstream/info', { params: { path } }))
			]);
			streams = s as unknown as StreamInfo[];
			info = i as unknown as AccountInfo;
			error = null;
		} catch (e) {
			error = errMsg(e);
		}
	}

	onMount(() => {
		load();
		const t = setInterval(() => !document.hidden && !createOpen && load(), 5000);
		return () => clearInterval(t);
	});

	async function create(cfg: StreamConfig) {
		creating = true;
		try {
			await unwrap(api.POST('/v1/systems/{system}/accounts/{account}/jetstream/streams', { params: { path }, body: cfg as never }));
			toast(`Created stream ${cfg.name}`);
			createOpen = false;
			goto(`/systems/${path.system}/a/${path.account}/streams/${encodeURIComponent(cfg.name)}`);
		} catch (e) {
			toast(errMsg(e), 'bad');
		} finally {
			creating = false;
		}
	}
</script>

<svelte:head><title>Streams · natsmin</title></svelte:head>

<div class="page" style="max-width:none">
	<AccountHead title="Streams" sub={info ? `${info.streams} streams · ${info.consumers} consumers${info.domain ? ` · domain ${info.domain}` : ''}` : undefined}>
		{#snippet actions()}
			<input type="search" placeholder="Filter by name or subject…" bind:value={filter} style="width:240px" />
			{#if can('operator')}<button class="btn primary" onclick={() => (createOpen = true)}><Icon name="plus" />New stream</button>{/if}
		{/snippet}
	</AccountHead>

	{#if info}
		<div class="kpis" style="margin-bottom:14px">
			<div class="card usage">
				<div class="row"><span class="label">File storage</span><span class="spacer"></span><span class="num">{bytes(info.storage)}{info.limits.max_storage > 0 ? ` / ${bytes(info.limits.max_storage)}` : ''}</span></div>
				{#if info.limits.max_storage > 0}<Meter used={info.storage} total={info.limits.max_storage} />{:else}<span class="faint" style="font-size:12px">no account limit</span>{/if}
			</div>
			<div class="card usage">
				<div class="row"><span class="label">Memory storage</span><span class="spacer"></span><span class="num">{bytes(info.memory)}{info.limits.max_memory > 0 ? ` / ${bytes(info.limits.max_memory)}` : ''}</span></div>
				{#if info.limits.max_memory > 0}<Meter used={info.memory} total={info.limits.max_memory} />{:else}<span class="faint" style="font-size:12px">no account limit</span>{/if}
			</div>
			<div class="card usage">
				<div class="row"><span class="label">Streams</span><span class="spacer"></span><span class="num">{info.streams}{info.limits.max_streams > 0 ? ` / ${info.limits.max_streams}` : ''}</span></div>
				<div class="row"><span class="label">Consumers</span><span class="spacer"></span><span class="num">{info.consumers}{info.limits.max_consumers > 0 ? ` / ${info.limits.max_consumers}` : ''}</span></div>
			</div>
			<div class="card usage">
				<div class="row"><span class="label">API calls</span><span class="spacer"></span><span class="num">{compact(info.api.total)}</span></div>
				<div class="row"><span class="label">API errors</span><span class="spacer"></span><span class="num">{num(info.api.errors)}</span></div>
			</div>
		</div>
	{/if}

	{#if error}
		<div class="alert bad">{error}</div>
	{:else if !streams}
		<Loading />
	{:else if streams.length === 0}
		<div class="card">
			<Empty icon="stream" title="No streams yet">
				Streams store messages published to their subjects.
				{#if can('operator')}<div style="margin-top:12px"><button class="btn primary" onclick={() => (createOpen = true)}><Icon name="plus" />Create a stream</button></div>{/if}
			</Empty>
		</div>
	{:else}
		<div class="card table-wrap">
			<table class="t">
				<thead>
					<tr>
						<SortTh {sorter} key="config.name">Stream</SortTh>
						<th>Subjects</th>
						<SortTh {sorter} key="config.storage">Storage</SortTh>
						<SortTh {sorter} key="config.num_replicas">Replicas</SortTh>
						<SortTh {sorter} key="cluster.name">Cluster / leader</SortTh>
						<SortTh {sorter} key="state.messages" right desc>Messages</SortTh>
						<SortTh {sorter} key="state.bytes" right desc>Size</SortTh>
						<SortTh {sorter} key="state.consumer_count" right desc>Consumers</SortTh>
						<SortTh {sorter} key="state.last_ts" desc>Last message</SortTh>
					</tr>
				</thead>
				<tbody>
					{#each rows as s (s.config.name)}
						<tr class="clickable" onclick={() => goto(`/systems/${path.system}/a/${path.account}/streams/${encodeURIComponent(s.config.name)}`)}>
							<td>
								<b>{s.config.name}</b>
								{#if s.config.sealed}<span class="badge">sealed</span>{/if}
								{#if s.config.mirror}<span class="badge info">mirror</span>{/if}
								{#if s.config.retention && s.config.retention !== 'limits'}<span class="badge">{s.config.retention}</span>{/if}
								{#if s.config.description}<div class="faint" style="font-size:12px">{s.config.description}</div>{/if}
							</td>
							<td class="mono" style="font-size:12px">{(s.config.subjects ?? []).join(', ') || (s.config.mirror ? `← ${s.config.mirror.name}` : '—')}</td>
							<td>{s.config.storage}</td>
							<td>
								R{s.config.num_replicas}
								{#if s.cluster?.replicas?.some((r) => !r.current || r.offline)}<span class="badge warn">lagging</span>{/if}
							</td>
							<td>{s.cluster?.name ?? '—'}<div class="faint" style="font-size:12px">{s.cluster?.leader ?? ''}</div></td>
							<td class="r num">{num(s.state.messages)}</td>
							<td class="r num">{bytes(s.state.bytes)}</td>
							<td class="r num">{s.state.consumer_count}</td>
							<td class="nowrap">{s.state.messages ? ago(s.state.last_ts) : '—'}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>

<Modal bind:open={createOpen} title="New stream" width={820}>
	<StreamForm {clusters} onsubmit={create} busy={creating} />
</Modal>

<style>
	.usage {
		padding: 12px 14px;
		display: flex;
		flex-direction: column;
		gap: 6px;
		font-size: 13px;
	}
</style>
