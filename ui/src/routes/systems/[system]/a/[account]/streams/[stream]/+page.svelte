<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api, unwrap, errMsg } from '$lib/api/client';
	import { can } from '$lib/session.svelte';
	import { toast } from '$lib/toast.svelte';
	import { getLive } from '$lib/context';
	import { confirm } from '$lib/components/Confirm.svelte';
	import type { StreamInfo, ConsumerInfo, StreamConfig, ConsumerConfig } from '$lib/nats';
	import { bytes, compact, dateTime, ago, nanos, num, toGoDuration } from '$lib/format';
	import AccountHead from '$lib/AccountHead.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import Loading from '$lib/components/Loading.svelte';
	import Empty from '$lib/components/Empty.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import JsonView from '$lib/components/JsonView.svelte';
	import StreamForm from '$lib/components/StreamForm.svelte';
	import ConsumerForm from '$lib/components/ConsumerForm.svelte';
	import MessageBrowser from '$lib/components/MessageBrowser.svelte';

	type Tab = 'overview' | 'messages' | 'consumers' | 'subjects' | 'config' | 'json';
	const live = getLive();
	const path = $derived({ system: page.params.system!, account: page.params.account!, stream: page.params.stream! });
	const base = $derived(`/systems/${path.system}/a/${path.account}/streams`);
	let tab = $state<Tab>((page.url.searchParams.get('tab') as Tab) || 'overview');
	let info = $state<StreamInfo | null>(null);
	let consumers = $state<ConsumerInfo[] | null>(null);
	let subjects = $state<[string, number][] | null>(null);
	let subjectFilter = $state('');
	let msgSubject = $state('');
	let error = $state<string | null>(null);
	let editOpen = $state(false);
	let purgeOpen = $state(false);
	let consumerOpen = $state(false);
	let busy = $state(false);

	async function load() {
		try {
			info = (await unwrap(api.GET('/v1/systems/{system}/accounts/{account}/jetstream/streams/{stream}', { params: { path } }))) as unknown as StreamInfo;
			error = null;
		} catch (e) {
			error = errMsg(e);
		}
	}
	async function loadConsumers() {
		try {
			consumers = (await unwrap(api.GET('/v1/systems/{system}/accounts/{account}/jetstream/streams/{stream}/consumers', { params: { path } }))) as unknown as ConsumerInfo[];
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}
	async function loadSubjects() {
		try {
			const r = (await unwrap(
				api.GET('/v1/systems/{system}/accounts/{account}/jetstream/streams/{stream}', { params: { path, query: { subjects: '>' } } })
			)) as unknown as StreamInfo;
			subjects = Object.entries(r.state.subjects ?? {}).sort((a, b) => b[1] - a[1]);
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}

	onMount(() => {
		load();
		loadConsumers();
		const t = setInterval(() => {
			if (document.hidden || editOpen || purgeOpen || consumerOpen) return;
			load();
			if (tab === 'consumers' || tab === 'overview') loadConsumers();
		}, 5000);
		return () => clearInterval(t);
	});

	$effect(() => {
		if (tab === 'subjects' && !subjects) loadSubjects();
	});

	function setTab(t: Tab) {
		tab = t;
		const u = new URL(page.url);
		u.searchParams.set('tab', t);
		history.replaceState(history.state, '', u);
	}

	async function save(cfg: StreamConfig) {
		busy = true;
		try {
			info = (await unwrap(api.PUT('/v1/systems/{system}/accounts/{account}/jetstream/streams/{stream}', { params: { path }, body: cfg as never }))) as unknown as StreamInfo;
			toast('Stream updated');
			editOpen = false;
		} catch (e) {
			toast(errMsg(e), 'bad');
		} finally {
			busy = false;
		}
	}

	async function remove() {
		if (
			!(await confirm({
				title: 'Delete stream',
				message: `Delete ${path.stream} with ${num(info?.state.messages)} messages and ${info?.state.consumer_count} consumers? This cannot be undone.`,
				confirmText: 'Delete stream',
				danger: true,
				typeToConfirm: path.stream
			}))
		)
			return;
		try {
			await unwrap(api.DELETE('/v1/systems/{system}/accounts/{account}/jetstream/streams/{stream}', { params: { path } }));
			toast(`Deleted ${path.stream}`);
			goto(base);
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}

	let purgeMode = $state<'all' | 'subject' | 'keep' | 'seq'>('all');
	let purgeSubject = $state('');
	let purgeKeep = $state('');
	let purgeSeq = $state('');
	async function purge() {
		const body: { subject?: string; keep?: number; seq?: number } = {};
		if (purgeMode === 'subject') body.subject = purgeSubject;
		if (purgeMode === 'keep') body.keep = parseInt(purgeKeep, 10);
		if (purgeMode === 'seq') body.seq = parseInt(purgeSeq, 10);
		busy = true;
		try {
			const r = (await unwrap(api.POST('/v1/systems/{system}/accounts/{account}/jetstream/streams/{stream}/purge', { params: { path }, body }))) as { purged?: number };
			toast(`Purged ${num(r.purged ?? 0)} messages`);
			purgeOpen = false;
			load();
			subjects = null;
		} catch (e) {
			toast(errMsg(e), 'bad');
		} finally {
			busy = false;
		}
	}

	async function stepdown() {
		try {
			await unwrap(api.POST('/v1/systems/{system}/accounts/{account}/jetstream/streams/{stream}/stepdown', { params: { path } }));
			toast('Leader stepped down');
			setTimeout(load, 1500);
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}

	async function createConsumer(cfg: ConsumerConfig) {
		busy = true;
		try {
			const c = (await unwrap(
				api.POST('/v1/systems/{system}/accounts/{account}/jetstream/streams/{stream}/consumers', { params: { path }, body: cfg as never })
			)) as unknown as ConsumerInfo;
			toast(`Created consumer ${c.name}`);
			consumerOpen = false;
			loadConsumers();
		} catch (e) {
			toast(errMsg(e), 'bad');
		} finally {
			busy = false;
		}
	}

	const cfg = $derived(info?.config);
	const limit = (n: number | undefined, f: (n: number) => string = num) => (n == null || n <= 0 ? 'unlimited' : f(n));
</script>

<svelte:head><title>{path.stream} · Streams · natsmin</title></svelte:head>

<div class="page" style="max-width:none">
	<AccountHead title={path.stream} sub={cfg?.description} crumbs={[['Streams', base]]}>
		{#snippet actions()}
			{#if can('operator') && info}
				<button class="btn" onclick={() => (consumerOpen = true)}><Icon name="plus" />Consumer</button>
				<button class="btn" onclick={() => (editOpen = true)}><Icon name="edit" />Edit</button>
				<button class="btn" onclick={() => (purgeOpen = true)} disabled={cfg?.deny_purge}><Icon name="filter" />Purge</button>
				<button class="btn danger" onclick={remove}><Icon name="trash" />Delete</button>
			{/if}
		{/snippet}
	</AccountHead>

	{#if error}
		<div class="alert bad">{error}</div>
	{:else if !info || !cfg}
		<Loading />
	{:else}
		<div class="tabs">
			{#each [['overview', 'Overview'], ['messages', 'Messages'], ['consumers', `Consumers (${info.state.consumer_count})`], ['subjects', 'Subjects'], ['config', 'Configuration'], ['json', 'JSON']] as [k, label]}
				<button class:active={tab === k} onclick={() => setTab(k as Tab)}>{label}</button>
			{/each}
		</div>

		{#if tab === 'overview'}
			<div class="kpis">
				<div class="card kpi"><span class="label">Messages</span><b class="num">{num(info.state.messages)}</b></div>
				<div class="card kpi"><span class="label">Size</span><b class="num">{bytes(info.state.bytes)}</b></div>
				<div class="card kpi"><span class="label">Sequence range</span><b class="num">{num(info.state.first_seq)} – {num(info.state.last_seq)}</b></div>
				<div class="card kpi"><span class="label">Last message</span><b>{info.state.messages ? ago(info.state.last_ts) : '—'}</b></div>
				<div class="card kpi"><span class="label">Subjects</span><b class="num">{num(info.state.num_subjects ?? 0)}</b></div>
				<div class="card kpi"><span class="label">Deleted</span><b class="num">{num(info.state.num_deleted ?? 0)}</b></div>
			</div>

			<div class="grid-2" style="margin-top:14px">
				<div class="card">
					<div class="card-head">
						<h2>Replicas</h2>
						<span class="badge">R{cfg.num_replicas}</span>
						{#if info.cluster?.name}<span class="faint">cluster {info.cluster.name}</span>{/if}
						<span class="spacer"></span>
						{#if can('operator') && (cfg.num_replicas ?? 1) > 1}<button class="btn sm" onclick={stepdown}><Icon name="swap" size={13} />Step down leader</button>{/if}
					</div>
					<table class="t">
						<thead><tr><th>Server</th><th>Role</th><th>State</th><th class="r">Lag</th><th>Last seen</th></tr></thead>
						<tbody>
							{#if info.cluster?.leader}
								<tr><td><b>{info.cluster.leader}</b></td><td><span class="badge accent">leader</span></td><td><span class="badge ok">current</span></td><td class="r">0</td><td>—</td></tr>
							{/if}
							{#each info.cluster?.replicas ?? [] as r}
								<tr>
									<td>{r.name}</td>
									<td>follower</td>
									<td>{#if r.offline}<span class="badge bad">offline</span>{:else if r.current}<span class="badge ok">current</span>{:else}<span class="badge warn">catching up</span>{/if}</td>
									<td class="r num">{num(r.lag ?? 0)}</td>
									<td>{nanos(r.active)} ago</td>
								</tr>
							{/each}
						</tbody>
					</table>
					{#if info.mirror || info.sources?.length}
						<div class="card-body">
							<h3 style="margin-bottom:6px">Replication</h3>
							{#each info.mirror ? [{ ...info.mirror, kind: 'mirror' }] : [] as m}
								<div class="row"><span class="badge info">mirror</span>{m.name}<span class="spacer"></span>lag {num(m.lag)} · active {nanos(m.active)} ago</div>
							{/each}
							{#each info.sources ?? [] as s}
								<div class="row"><span class="badge info">source</span>{s.name}<span class="spacer"></span>lag {num(s.lag)} · active {nanos(s.active)} ago</div>
							{/each}
						</div>
					{/if}
				</div>

				<div class="card">
					<div class="card-head"><h2>Consumers</h2><span class="spacer"></span><button class="btn sm" onclick={() => setTab('consumers')}>All</button></div>
					{#if !consumers}<Loading />
					{:else if consumers.length === 0}<Empty icon="users" title="No consumers" />
					{:else}
						<table class="t">
							<thead><tr><th>Consumer</th><th class="r">Pending</th><th class="r">Ack pending</th><th class="r">Redelivered</th></tr></thead>
							<tbody>
								{#each consumers.slice(0, 8) as c}
									<tr class="clickable" onclick={() => goto(`${base}/${encodeURIComponent(path.stream)}/consumers/${encodeURIComponent(c.name)}`)}>
										<td><b>{c.name}</b>{#if c.paused}<span class="badge warn">paused</span>{/if}</td>
										<td class="r num">{num(c.num_pending)}</td>
										<td class="r num">{num(c.num_ack_pending)}</td>
										<td class="r num">{num(c.num_redelivered)}</td>
									</tr>
								{/each}
							</tbody>
						</table>
					{/if}
				</div>
			</div>

			<div class="card" style="margin-top:14px">
				<div class="card-head"><h2>Summary</h2></div>
				<div class="card-body grid-3">
					<dl class="kv-list">
						<dt>Subjects</dt><dd class="mono">{(cfg.subjects ?? []).join(', ') || '—'}</dd>
						<dt>Retention</dt><dd>{cfg.retention}</dd>
						<dt>Storage</dt><dd>{cfg.storage}{cfg.compression === 's2' ? ' · s2' : ''}</dd>
						<dt>Created</dt><dd>{dateTime(info.created)}</dd>
					</dl>
					<dl class="kv-list">
						<dt>Max messages</dt><dd>{limit(cfg.max_msgs)}</dd>
						<dt>Max size</dt><dd>{limit(cfg.max_bytes, bytes)}</dd>
						<dt>Max age</dt><dd>{cfg.max_age ? nanos(cfg.max_age) : 'unlimited'}</dd>
						<dt>Per subject</dt><dd>{limit(cfg.max_msgs_per_subject)}</dd>
					</dl>
					<dl class="kv-list">
						<dt>Discard</dt><dd>{cfg.discard}</dd>
						<dt>Duplicates</dt><dd>{cfg.duplicate_window ? nanos(cfg.duplicate_window) + ' window' : 'off'}</dd>
						<dt>Placement</dt><dd>{cfg.placement?.cluster ?? '—'}{cfg.placement?.tags?.length ? ` [${cfg.placement.tags.join(', ')}]` : ''}</dd>
						<dt>Direct get</dt><dd>{cfg.allow_direct ? 'yes' : 'no'}</dd>
					</dl>
				</div>
			</div>
		{:else if tab === 'messages'}
			<div class="card"><div class="card-body">
				<MessageBrowser
					system={path.system}
					account={path.account}
					stream={path.stream}
					firstSeq={info.state.first_seq}
					lastSeq={info.state.last_seq}
					bind:subject={msgSubject}
				/>
			</div></div>
		{:else if tab === 'consumers'}
			<div class="card table-wrap">
				{#if !consumers}<Loading />
				{:else if consumers.length === 0}
					<Empty icon="users" title="No consumers">
						{#if can('operator')}<button class="btn primary" style="margin-top:10px" onclick={() => (consumerOpen = true)}><Icon name="plus" />Create consumer</button>{/if}
					</Empty>
				{:else}
					<table class="t">
						<thead>
							<tr><th>Consumer</th><th>Type</th><th>Filter</th><th>Ack</th><th class="r">Pending</th><th class="r">Ack pending</th><th class="r">Redelivered</th><th class="r">Waiting</th><th>Last delivery</th><th>Leader</th></tr>
						</thead>
						<tbody>
							{#each consumers as c (c.name)}
								<tr class="clickable" onclick={() => goto(`${base}/${encodeURIComponent(path.stream)}/consumers/${encodeURIComponent(c.name)}`)}>
									<td>
										<b>{c.name}</b>
										{#if c.paused}<span class="badge warn">paused</span>{/if}
										{#if !c.config.durable_name}<span class="badge">ephemeral</span>{/if}
									</td>
									<td>{c.config.deliver_subject ? 'push' : 'pull'}</td>
									<td class="mono" style="font-size:12px">{c.config.filter_subject ?? (c.config.filter_subjects ?? []).join(', ') ?? ''}</td>
									<td>{c.config.ack_policy}</td>
									<td class="r num">{num(c.num_pending)}</td>
									<td class="r num">{#if c.num_ack_pending > 0}<span class="badge warn">{num(c.num_ack_pending)}</span>{:else}0{/if}</td>
									<td class="r num">{num(c.num_redelivered)}</td>
									<td class="r num">{num(c.num_waiting)}</td>
									<td class="nowrap">{ago(c.delivered.last_active)}</td>
									<td>{c.cluster?.leader ?? '—'}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				{/if}
			</div>
		{:else if tab === 'subjects'}
			<div class="card">
				<div class="card-head">
					<h2>Subjects</h2>
					{#if subjects}<span class="badge">{num(subjects.length)}</span>{/if}
					<span class="spacer"></span>
					<input type="search" placeholder="Filter…" bind:value={subjectFilter} style="width:220px" />
					<button class="btn sm" onclick={loadSubjects}><Icon name="refresh" size={13} /></button>
				</div>
				{#if !subjects}<Loading />
				{:else}
					{@const shown = subjects.filter(([s]) => !subjectFilter || s.includes(subjectFilter))}
					<div class="table-wrap" style="max-height:640px">
						<table class="t">
							<thead><tr><th>Subject</th><th class="r">Messages</th><th></th></tr></thead>
							<tbody>
								{#each shown.slice(0, 2000) as [s, n]}
									<tr>
										<td class="mono">{s}</td>
										<td class="r num">{num(n)}</td>
										<td class="r"><button class="btn sm" onclick={() => ((msgSubject = s), setTab('messages'))}>Browse</button></td>
									</tr>
								{/each}
							</tbody>
						</table>
					</div>
					{#if shown.length > 2000}<div class="card-body faint">Showing 2,000 of {num(shown.length)}; filter to narrow down.</div>{/if}
				{/if}
			</div>
		{:else if tab === 'config'}
			<div class="card"><div class="card-body">
				<dl class="kv-list">
					{#each Object.entries(cfg) as [k, v]}
						<dt class="mono">{k}</dt>
						<dd class="mono">
							{#if typeof v === 'object'}{JSON.stringify(v)}
							{:else if ['max_age', 'duplicate_window', 'subject_delete_marker_ttl'].includes(k) && typeof v === 'number'}{v ? toGoDuration(v) : '0'} <span class="faint">({v} ns)</span>
							{:else}{String(v)}{/if}
						</dd>
					{/each}
				</dl>
			</div></div>
		{:else}
			<JsonView value={info} maxHeight={800} />
		{/if}
	{/if}
</div>

{#if info}
	<Modal bind:open={editOpen} title="Edit {path.stream}" width={820}>
		<StreamForm initial={info.config} clusters={live.snapshot?.clusters.map((c) => c.name) ?? []} onsubmit={save} {busy} submitText="Save changes" />
	</Modal>
	<Modal bind:open={consumerOpen} title="New consumer on {path.stream}" width={780}>
		<ConsumerForm onsubmit={createConsumer} {busy} />
	</Modal>
	<Modal bind:open={purgeOpen} title="Purge {path.stream}" width={500}>
		<div class="stack">
			<label class="check"><input type="radio" bind:group={purgeMode} value="all" />Every message ({num(info.state.messages)})</label>
			<label class="check"><input type="radio" bind:group={purgeMode} value="subject" />Messages on a subject</label>
			{#if purgeMode === 'subject'}<input bind:value={purgeSubject} placeholder="orders.eu.>" />{/if}
			<label class="check"><input type="radio" bind:group={purgeMode} value="keep" />Keep only the newest N</label>
			{#if purgeMode === 'keep'}<input bind:value={purgeKeep} placeholder="1000" />{/if}
			<label class="check"><input type="radio" bind:group={purgeMode} value="seq" />Everything before a sequence</label>
			{#if purgeMode === 'seq'}<input bind:value={purgeSeq} placeholder={String(info.state.last_seq)} />{/if}
		</div>
		{#snippet footer()}
			<button class="btn" onclick={() => (purgeOpen = false)}>Cancel</button>
			<button class="btn danger solid" onclick={purge} disabled={busy}>Purge</button>
		{/snippet}
	</Modal>
{/if}

<style>
	.kpi {
		padding: 12px 14px;
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	.kpi b {
		font-size: 18px;
		font-weight: 600;
	}
</style>
