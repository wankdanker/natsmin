<script lang="ts">
	import { untrack } from 'svelte';
	import type { ConsumerConfig } from '$lib/nats';
	import { parseGoDuration, toGoDuration } from '$lib/format';
	import JsonEditor from './JsonEditor.svelte';

	let {
		initial = null,
		onsubmit,
		busy = false,
		submitText = 'Create consumer'
	}: { initial?: ConsumerConfig | null; onsubmit: (cfg: ConsumerConfig) => void; busy?: boolean; submitText?: string } = $props();

	const init = untrack(() => initial);
	const editing = !!init;
	const base: ConsumerConfig = init ? structuredClone($state.snapshot(init)) : {};
	let mode = $state<'form' | 'json'>('form');
	let name = $state(base.durable_name ?? base.name ?? '');
	let durable = $state(init ? !!base.durable_name || !base.inactive_threshold : true);
	let description = $state(base.description ?? '');
	let kind = $state<'pull' | 'push'>(base.deliver_subject ? 'push' : 'pull');
	let deliverPolicy = $state(base.deliver_policy ?? 'all');
	let startSeq = $state(base.opt_start_seq ? String(base.opt_start_seq) : '');
	let startTime = $state(base.opt_start_time ? base.opt_start_time.slice(0, 19) : '');
	let ackPolicy = $state(base.ack_policy ?? 'explicit');
	let ackWait = $state(toGoDuration(base.ack_wait) || '30s');
	let maxDeliver = $state(base.max_deliver && base.max_deliver > 0 ? String(base.max_deliver) : '');
	let filters = $state((base.filter_subjects ?? (base.filter_subject ? [base.filter_subject] : [])).join('\n'));
	let replay = $state(base.replay_policy ?? 'instant');
	let maxAckPending = $state(base.max_ack_pending ? String(base.max_ack_pending) : '');
	let maxWaiting = $state(base.max_waiting ? String(base.max_waiting) : '');
	let inactive = $state(toGoDuration(base.inactive_threshold));
	let replicas = $state(base.num_replicas ?? 0);
	let memStorage = $state(base.mem_storage ?? false);
	let headersOnly = $state(base.headers_only ?? false);
	let backoff = $state((base.backoff ?? []).map((b) => toGoDuration(b)).join(', '));
	let deliverSubject = $state(base.deliver_subject ?? '');
	let deliverGroup = $state(base.deliver_group ?? '');
	let flowControl = $state(base.flow_control ?? false);
	let heartbeat = $state(toGoDuration(base.idle_heartbeat));
	let json = $state('');
	let jsonError = $state<string | null>(null);
	let formError = $state<string | null>(null);

	function dur(v: string, label: string) {
		const d = parseGoDuration(v);
		if (d === null) throw new Error(`${label} must be a duration like 30s`);
		return d || undefined;
	}

	function build(): ConsumerConfig {
		const cfg: ConsumerConfig = { ...base };
		delete cfg.durable_name;
		delete cfg.name;
		if (name) {
			if (durable) cfg.durable_name = name;
			else cfg.name = name;
		}
		cfg.description = description || undefined;
		cfg.deliver_policy = deliverPolicy as ConsumerConfig['deliver_policy'];
		cfg.opt_start_seq = deliverPolicy === 'by_start_sequence' ? parseInt(startSeq, 10) || 1 : undefined;
		cfg.opt_start_time = deliverPolicy === 'by_start_time' && startTime ? new Date(startTime).toISOString() : undefined;
		cfg.ack_policy = ackPolicy as ConsumerConfig['ack_policy'];
		cfg.ack_wait = ackPolicy === 'none' ? undefined : dur(ackWait, 'Ack wait');
		cfg.max_deliver = maxDeliver ? parseInt(maxDeliver, 10) : -1;
		const fs = filters.split(/[\n,]/).map((s) => s.trim()).filter(Boolean);
		delete cfg.filter_subject;
		delete cfg.filter_subjects;
		if (fs.length === 1) cfg.filter_subject = fs[0];
		else if (fs.length > 1) cfg.filter_subjects = fs;
		cfg.replay_policy = replay as ConsumerConfig['replay_policy'];
		cfg.max_ack_pending = maxAckPending ? parseInt(maxAckPending, 10) : undefined;
		cfg.inactive_threshold = dur(inactive, 'Inactive threshold');
		cfg.num_replicas = Number(replicas) || undefined;
		cfg.mem_storage = memStorage || undefined;
		cfg.headers_only = headersOnly || undefined;
		const bo = backoff.split(',').map((s) => s.trim()).filter(Boolean);
		cfg.backoff = bo.length ? bo.map((b) => dur(b, 'Backoff')!) : undefined;
		if (kind === 'push') {
			cfg.deliver_subject = deliverSubject;
			cfg.deliver_group = deliverGroup || undefined;
			cfg.flow_control = flowControl || undefined;
			cfg.idle_heartbeat = dur(heartbeat, 'Idle heartbeat');
			delete cfg.max_waiting;
		} else {
			delete cfg.deliver_subject;
			delete cfg.deliver_group;
			delete cfg.flow_control;
			delete cfg.idle_heartbeat;
			cfg.max_waiting = maxWaiting ? parseInt(maxWaiting, 10) : undefined;
		}
		return JSON.parse(JSON.stringify(cfg));
	}

	function toJSON() {
		try {
			json = JSON.stringify(build(), null, 2);
			formError = null;
			mode = 'json';
		} catch (e) {
			formError = (e as Error).message;
		}
	}

	function submit(e: SubmitEvent) {
		e.preventDefault();
		try {
			formError = null;
			onsubmit(mode === 'json' ? JSON.parse(json) : build());
		} catch (err) {
			formError = (err as Error).message;
		}
	}
</script>

<form onsubmit={submit} class="stack">
	<div class="tabs" style="margin-bottom:0">
		<button type="button" class:active={mode === 'form'} onclick={() => (mode = 'form')}>Form</button>
		<button type="button" class:active={mode === 'json'} onclick={toJSON}>JSON</button>
	</div>
	{#if formError}<div class="alert bad">{formError}</div>{/if}
	{#if mode === 'json'}
		<JsonEditor bind:value={json} bind:error={jsonError} rows={22} />
	{:else}
		<div class="grid-2">
			<label class="field"><span>Name</span><input bind:value={name} disabled={editing} placeholder="billing-worker" /></label>
			<label class="field"><span>Description</span><input bind:value={description} /></label>
		</div>
		<div class="row">
			<label class="check"><input type="checkbox" bind:checked={durable} disabled={editing} />Durable (survives when no client is bound)</label>
			<span class="spacer"></span>
			<div class="tabs" style="margin:0;border:none">
				<button type="button" class:active={kind === 'pull'} onclick={() => (kind = 'pull')} disabled={editing}>Pull</button>
				<button type="button" class:active={kind === 'push'} onclick={() => (kind = 'push')} disabled={editing}>Push</button>
			</div>
		</div>
		<label class="field">
			<span>Filter subjects</span>
			<textarea rows="2" bind:value={filters} placeholder="orders.eu.>"></textarea>
			<span class="hint">One per line; empty receives everything in the stream.</span>
		</label>
		<div class="grid-3">
			<label class="field">
				<span>Start from</span>
				<select bind:value={deliverPolicy} disabled={editing}>
					<option value="all">First message</option>
					<option value="last">Last message</option>
					<option value="new">New messages only</option>
					<option value="last_per_subject">Last per subject</option>
					<option value="by_start_sequence">Sequence…</option>
					<option value="by_start_time">Time…</option>
				</select>
			</label>
			{#if deliverPolicy === 'by_start_sequence'}
				<label class="field"><span>Start sequence</span><input bind:value={startSeq} disabled={editing} /></label>
			{:else if deliverPolicy === 'by_start_time'}
				<label class="field"><span>Start time</span><input type="datetime-local" step="1" bind:value={startTime} disabled={editing} /></label>
			{/if}
			<label class="field">
				<span>Acknowledgement</span>
				<select bind:value={ackPolicy} disabled={editing}>
					<option value="explicit">Explicit (each message)</option>
					<option value="all">All (cumulative)</option>
					<option value="none">None</option>
				</select>
			</label>
			{#if ackPolicy !== 'none'}
				<label class="field"><span>Ack wait</span><input bind:value={ackWait} /></label>
			{/if}
			<label class="field"><span>Max deliveries</span><input bind:value={maxDeliver} placeholder="unlimited" /></label>
			<label class="field"><span>Max ack pending</span><input bind:value={maxAckPending} placeholder="server default" /></label>
			<label class="field"
				><span>Replay</span><select bind:value={replay} disabled={editing}><option value="instant">Instant</option><option value="original">Original timing</option></select
				></label
			>
			<label class="field"><span>Backoff</span><input bind:value={backoff} placeholder="1s, 5s, 30s" /></label>
			<label class="field"><span>Inactive threshold</span><input bind:value={inactive} placeholder={durable ? 'never' : '5s'} /></label>
			<label class="field"
				><span>Replicas</span><select bind:value={replicas}
					><option value={0}>Same as stream</option>{#each [1, 2, 3, 4, 5] as r}<option value={r}>{r}</option>{/each}</select
				></label
			>
			{#if kind === 'pull'}
				<label class="field"><span>Max waiting pulls</span><input bind:value={maxWaiting} placeholder="512" /></label>
			{/if}
		</div>
		{#if kind === 'push'}
			<div class="grid-3">
				<label class="field"><span>Deliver subject</span><input bind:value={deliverSubject} required disabled={editing} /></label>
				<label class="field"><span>Queue group</span><input bind:value={deliverGroup} disabled={editing} /></label>
				<label class="field"><span>Idle heartbeat</span><input bind:value={heartbeat} placeholder="off" /></label>
			</div>
			<label class="check"><input type="checkbox" bind:checked={flowControl} />Flow control</label>
		{/if}
		<div class="row">
			<label class="check"><input type="checkbox" bind:checked={memStorage} disabled={editing} />Memory storage for consumer state</label>
			<label class="check"><input type="checkbox" bind:checked={headersOnly} />Headers only</label>
		</div>
	{/if}
	<div class="row"><button class="btn primary" disabled={busy || (mode === 'json' && !!jsonError)}>{busy ? 'Saving…' : submitText}</button></div>
</form>
