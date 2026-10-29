<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api, unwrap, errMsg } from '$lib/api/client';
	import { can } from '$lib/session.svelte';
	import { toast } from '$lib/toast.svelte';
	import { confirm } from '$lib/components/Confirm.svelte';
	import type { ConsumerInfo, ConsumerConfig } from '$lib/nats';
	import { ago, compact, dateTime, nanos, num, toGoDuration } from '$lib/format';
	import AccountHead from '$lib/AccountHead.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import Loading from '$lib/components/Loading.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import JsonView from '$lib/components/JsonView.svelte';
	import Chart from '$lib/components/Chart.svelte';
	import ConsumerForm from '$lib/components/ConsumerForm.svelte';

	const path = $derived({ system: page.params.system!, account: page.params.account!, stream: page.params.stream!, consumer: page.params.consumer! });
	const streamHref = $derived(`/systems/${path.system}/a/${path.account}/streams/${encodeURIComponent(path.stream)}`);
	let info = $state<ConsumerInfo | null>(null);
	let error = $state<string | null>(null);
	let hist = $state<{ t: number; pending: number; ackPending: number; redelivered: number }[]>([]);
	let editOpen = $state(false);
	let pauseOpen = $state(false);
	let pauseFor = $state('1h');
	let busy = $state(false);

	async function load() {
		try {
			info = (await unwrap(api.GET('/v1/systems/{system}/accounts/{account}/jetstream/streams/{stream}/consumers/{consumer}', { params: { path } }))) as unknown as ConsumerInfo;
			hist.push({ t: Date.now(), pending: info.num_pending, ackPending: info.num_ack_pending, redelivered: info.num_redelivered });
			if (hist.length > 300) hist.shift();
			error = null;
		} catch (e) {
			error = errMsg(e);
		}
	}
	onMount(() => {
		load();
		const t = setInterval(() => !document.hidden && !editOpen && load(), 3000);
		return () => clearInterval(t);
	});

	async function save(cfg: ConsumerConfig) {
		busy = true;
		try {
			info = (await unwrap(
				api.PUT('/v1/systems/{system}/accounts/{account}/jetstream/streams/{stream}/consumers/{consumer}', { params: { path }, body: cfg as never })
			)) as unknown as ConsumerInfo;
			toast('Consumer updated');
			editOpen = false;
		} catch (e) {
			toast(errMsg(e), 'bad');
		} finally {
			busy = false;
		}
	}

	async function pause() {
		const m = pauseFor.match(/^(\d+)([mhd])$/);
		const ms = m ? parseInt(m[1]) * { m: 6e4, h: 3.6e6, d: 8.64e7 }[m[2] as 'm' | 'h' | 'd'] : 3.6e6;
		try {
			await unwrap(
				api.POST('/v1/systems/{system}/accounts/{account}/jetstream/streams/{stream}/consumers/{consumer}/pause', {
					params: { path },
					body: { until: new Date(Date.now() + ms).toISOString() }
				})
			);
			toast('Consumer paused');
			pauseOpen = false;
			load();
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}
	async function resume() {
		try {
			await unwrap(api.POST('/v1/systems/{system}/accounts/{account}/jetstream/streams/{stream}/consumers/{consumer}/resume', { params: { path } }));
			toast('Consumer resumed');
			load();
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}
	async function stepdown() {
		try {
			await unwrap(api.POST('/v1/systems/{system}/accounts/{account}/jetstream/streams/{stream}/consumers/{consumer}/stepdown', { params: { path } }));
			toast('Leader stepped down');
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}
	async function remove() {
		if (!(await confirm({ title: 'Delete consumer', message: `Delete consumer ${path.consumer}? Clients bound to it stop receiving messages.`, confirmText: 'Delete', danger: true, typeToConfirm: path.consumer })))
			return;
		try {
			await unwrap(api.DELETE('/v1/systems/{system}/accounts/{account}/jetstream/streams/{stream}/consumers/{consumer}', { params: { path } }));
			toast(`Deleted ${path.consumer}`);
			goto(`${streamHref}?tab=consumers`);
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}

	const c = $derived(info?.config);
</script>

<svelte:head><title>{path.consumer} · natsmin</title></svelte:head>

<div class="page" style="max-width:none">
	<AccountHead title={path.consumer} sub={c?.description} crumbs={[['Streams', `/systems/${path.system}/a/${path.account}/streams`], [path.stream, `${streamHref}?tab=consumers`]]}>
		{#snippet actions()}
			{#if can('operator') && info}
				{#if info.paused}
					<button class="btn" onclick={resume}><Icon name="play" />Resume</button>
				{:else}
					<button class="btn" onclick={() => (pauseOpen = true)}><Icon name="pause" />Pause</button>
				{/if}
				{#if (info.cluster?.replicas?.length ?? 0) > 0}<button class="btn" onclick={stepdown}><Icon name="swap" />Step down</button>{/if}
				<button class="btn" onclick={() => (editOpen = true)}><Icon name="edit" />Edit</button>
				<button class="btn danger" onclick={remove}><Icon name="trash" />Delete</button>
			{/if}
		{/snippet}
	</AccountHead>

	{#if error}<div class="alert bad">{error}</div>
	{:else if !info || !c}<Loading />
	{:else}
		{#if info.paused}<div class="alert warn" style="margin-bottom:14px">Paused{c.pause_until ? ` until ${dateTime(c.pause_until)}` : ''}.</div>{/if}
		<div class="kpis">
			<div class="card kpi"><span class="label">Pending</span><b class="num">{num(info.num_pending)}</b><span class="faint">not yet delivered</span></div>
			<div class="card kpi"><span class="label">Ack pending</span><b class="num">{num(info.num_ack_pending)}</b><span class="faint">delivered, not acked{c.max_ack_pending ? ` · max ${num(c.max_ack_pending)}` : ''}</span></div>
			<div class="card kpi"><span class="label">Redelivered</span><b class="num">{num(info.num_redelivered)}</b></div>
			<div class="card kpi"><span class="label">Waiting pulls</span><b class="num">{num(info.num_waiting)}</b></div>
			<div class="card kpi"><span class="label">Delivered</span><b class="num">#{num(info.delivered.stream_seq)}</b><span class="faint">{ago(info.delivered.last_active)}</span></div>
			<div class="card kpi"><span class="label">Ack floor</span><b class="num">#{num(info.ack_floor.stream_seq)}</b><span class="faint">{ago(info.ack_floor.last_active)}</span></div>
		</div>

		<div class="grid-2" style="margin-top:14px">
			<div class="card">
				<div class="card-head"><h2>Backlog</h2><span class="faint" style="font-size:12px">since this page opened</span></div>
				<div class="card-body">
					<Chart
						times={hist.map((h) => h.t)}
						format={(n) => compact(n)}
						series={[
							{ name: 'pending', values: hist.map((h) => h.pending), color: 'var(--series-1)' },
							{ name: 'ack pending', values: hist.map((h) => h.ackPending), color: 'var(--series-3)' }
						]}
					/>
				</div>
			</div>
			<div class="card">
				<div class="card-head"><h2>Configuration</h2></div>
				<div class="card-body">
					<dl class="kv-list">
						<dt>Type</dt><dd>{c.deliver_subject ? `push → ${c.deliver_subject}${c.deliver_group ? ` (queue ${c.deliver_group})` : ''}` : 'pull'}{c.durable_name ? ' · durable' : ' · ephemeral'}</dd>
						<dt>Filter</dt><dd class="mono">{c.filter_subject ?? (c.filter_subjects ?? []).join(', ') ?? ''}{!c.filter_subject && !c.filter_subjects?.length ? 'all subjects' : ''}</dd>
						<dt>Deliver policy</dt><dd>{c.deliver_policy}{c.opt_start_seq ? ` from #${c.opt_start_seq}` : ''}{c.opt_start_time ? ` from ${dateTime(c.opt_start_time)}` : ''}</dd>
						<dt>Ack</dt><dd>{c.ack_policy}{c.ack_wait ? ` · wait ${toGoDuration(c.ack_wait)}` : ''}</dd>
						<dt>Max deliver</dt><dd>{c.max_deliver && c.max_deliver > 0 ? c.max_deliver : 'unlimited'}</dd>
						{#if c.backoff?.length}<dt>Backoff</dt><dd>{c.backoff.map((b) => toGoDuration(b)).join(', ')}</dd>{/if}
						<dt>Replay</dt><dd>{c.replay_policy}</dd>
						{#if c.inactive_threshold}<dt>Inactive threshold</dt><dd>{nanos(c.inactive_threshold)}</dd>{/if}
						<dt>Replicas</dt><dd>{c.num_replicas || 'same as stream'}{c.mem_storage ? ' · memory' : ''}</dd>
						<dt>Leader</dt><dd>{info.cluster?.leader ?? '—'}{(info.cluster?.replicas ?? []).map((r) => `, ${r.name}${r.current ? '' : ' (lagging)'}`).join('')}</dd>
						<dt>Created</dt><dd>{dateTime(info.created)}</dd>
					</dl>
				</div>
			</div>
		</div>
		<div style="margin-top:14px"><JsonView value={info} /></div>
	{/if}
</div>

{#if info}
	<Modal bind:open={editOpen} title="Edit {path.consumer}" width={780}>
		<ConsumerForm initial={info.config} onsubmit={save} {busy} submitText="Save changes" />
	</Modal>
	<Modal bind:open={pauseOpen} title="Pause {path.consumer}" width={420}>
		<label class="field">
			<span>Resume automatically after</span>
			<select bind:value={pauseFor}>
				<option value="5m">5 minutes</option><option value="30m">30 minutes</option><option value="1h">1 hour</option>
				<option value="6h">6 hours</option><option value="1d">1 day</option><option value="7d">7 days</option>
			</select>
		</label>
		{#snippet footer()}
			<button class="btn" onclick={() => (pauseOpen = false)}>Cancel</button>
			<button class="btn primary" onclick={pause}>Pause</button>
		{/snippet}
	</Modal>
{/if}

<style>
	.kpi {
		padding: 12px 14px;
		display: flex;
		flex-direction: column;
		gap: 1px;
	}
	.kpi b {
		font-size: 18px;
		font-weight: 600;
	}
	.kpi .faint {
		font-size: 12px;
	}
</style>
