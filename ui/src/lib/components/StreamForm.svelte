<script lang="ts">
	import { untrack } from 'svelte';
	import type { StreamConfig } from '$lib/nats';
	import { parseGoDuration, toGoDuration } from '$lib/format';
	import JsonEditor from './JsonEditor.svelte';

	let {
		initial = null,
		clusters = [],
		onsubmit,
		busy = false,
		submitText = 'Create stream'
	}: {
		initial?: StreamConfig | null;
		clusters?: string[];
		onsubmit: (cfg: StreamConfig) => void;
		busy?: boolean;
		submitText?: string;
	} = $props();

	const init = untrack(() => initial);
	const editing = !!init;
	let mode = $state<'form' | 'json'>('form');
	const start: StreamConfig = init ? structuredClone($state.snapshot(init)) : { name: '' };
	let base = $state<StreamConfig>(start);

	let name = $state(start.name ?? '');
	let description = $state(start.description ?? '');
	let subjects = $state((start.subjects ?? []).join('\n'));
	let storage = $state(start.storage ?? 'file');
	let replicas = $state(start.num_replicas ?? 1);
	let retention = $state(start.retention ?? 'limits');
	let discard = $state(start.discard ?? 'old');
	let maxMsgs = $state(start.max_msgs && start.max_msgs > 0 ? String(start.max_msgs) : '');
	let maxBytes = $state(start.max_bytes && start.max_bytes > 0 ? String(start.max_bytes) : '');
	let maxAge = $state(toGoDuration(start.max_age));
	let maxMsgSize = $state(start.max_msg_size && start.max_msg_size > 0 ? String(start.max_msg_size) : '');
	let maxPerSubject = $state(start.max_msgs_per_subject && start.max_msgs_per_subject > 0 ? String(start.max_msgs_per_subject) : '');
	let dupWindow = $state(toGoDuration(start.duplicate_window));
	let cluster = $state(start.placement?.cluster ?? '');
	let tags = $state((start.placement?.tags ?? []).join(', '));
	let compression = $state(start.compression === 's2');
	let allowDirect = $state(start.allow_direct ?? true);
	let allowRollup = $state(start.allow_rollup_hdrs ?? false);
	let denyDelete = $state(start.deny_delete ?? false);
	let denyPurge = $state(start.deny_purge ?? false);
	let allowTTL = $state(start.allow_msg_ttl ?? false);
	let json = $state('');
	let jsonError = $state<string | null>(null);
	let formError = $state<string | null>(null);

	function intOr(v: string, def: number) {
		const n = parseInt(v.replace(/[_,\s]/g, ''), 10);
		return isNaN(n) ? def : n;
	}

	function sizeOr(v: string, def: number) {
		const m = v.trim().match(/^(\d+(?:\.\d+)?)\s*(b|k|kb|kib|m|mb|mib|g|gb|gib|t|tb|tib)?$/i);
		if (!m) return def;
		const mult: Record<string, number> = { b: 1, k: 1024, kb: 1024, kib: 1024, m: 1024 ** 2, mb: 1024 ** 2, mib: 1024 ** 2, g: 1024 ** 3, gb: 1024 ** 3, gib: 1024 ** 3, t: 1024 ** 4, tb: 1024 ** 4, tib: 1024 ** 4 };
		return Math.round(parseFloat(m[1]) * (mult[(m[2] ?? 'b').toLowerCase()] ?? 1));
	}

	function build(): StreamConfig {
		const age = parseGoDuration(maxAge);
		const dup = parseGoDuration(dupWindow);
		if (age === null) throw new Error('Max age must be a duration like 24h or 30m');
		if (dup === null) throw new Error('Duplicate window must be a duration like 2m');
		const cfg: StreamConfig = {
			...base,
			name: name.trim(),
			description: description || undefined,
			subjects: subjects.split(/[\n,]/).map((s) => s.trim()).filter(Boolean),
			storage: storage as StreamConfig['storage'],
			num_replicas: Number(replicas),
			retention: retention as StreamConfig['retention'],
			discard: discard as StreamConfig['discard'],
			max_msgs: intOr(maxMsgs, -1),
			max_bytes: maxBytes ? sizeOr(maxBytes, -1) : -1,
			max_age: age,
			max_msg_size: maxMsgSize ? sizeOr(maxMsgSize, -1) : -1,
			max_msgs_per_subject: intOr(maxPerSubject, -1),
			duplicate_window: dup || undefined,
			compression: compression ? 's2' : 'none',
			allow_direct: allowDirect,
			allow_rollup_hdrs: allowRollup,
			deny_delete: denyDelete,
			deny_purge: denyPurge,
			allow_msg_ttl: allowTTL || undefined
		};
		if (cluster || tags.trim()) {
			cfg.placement = { cluster: cluster || undefined, tags: tags.split(',').map((t) => t.trim()).filter(Boolean) };
		} else {
			delete cfg.placement;
		}
		if (!cfg.subjects?.length && !cfg.mirror && !cfg.sources?.length) delete cfg.subjects;
		return cfg;
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

	function toForm() {
		try {
			const cfg = JSON.parse(json) as StreamConfig;
			base = cfg;
			name = cfg.name ?? '';
			description = cfg.description ?? '';
			subjects = (cfg.subjects ?? []).join('\n');
			storage = cfg.storage ?? 'file';
			replicas = cfg.num_replicas ?? 1;
			retention = cfg.retention ?? 'limits';
			discard = cfg.discard ?? 'old';
			maxMsgs = cfg.max_msgs && cfg.max_msgs > 0 ? String(cfg.max_msgs) : '';
			maxBytes = cfg.max_bytes && cfg.max_bytes > 0 ? String(cfg.max_bytes) : '';
			maxAge = toGoDuration(cfg.max_age);
			maxMsgSize = cfg.max_msg_size && cfg.max_msg_size > 0 ? String(cfg.max_msg_size) : '';
			maxPerSubject = cfg.max_msgs_per_subject && cfg.max_msgs_per_subject > 0 ? String(cfg.max_msgs_per_subject) : '';
			dupWindow = toGoDuration(cfg.duplicate_window);
			cluster = cfg.placement?.cluster ?? '';
			tags = (cfg.placement?.tags ?? []).join(', ');
			compression = cfg.compression === 's2';
			allowDirect = cfg.allow_direct ?? false;
			allowRollup = cfg.allow_rollup_hdrs ?? false;
			denyDelete = cfg.deny_delete ?? false;
			denyPurge = cfg.deny_purge ?? false;
			allowTTL = cfg.allow_msg_ttl ?? false;
			mode = 'form';
		} catch {}
	}

	function submit(e: SubmitEvent) {
		e.preventDefault();
		try {
			formError = null;
			onsubmit(mode === 'json' ? (JSON.parse(json) as StreamConfig) : build());
		} catch (err) {
			formError = (err as Error).message;
		}
	}
</script>

<form onsubmit={submit} class="stack">
	<div class="tabs" style="margin-bottom:0">
		<button type="button" class:active={mode === 'form'} onclick={toForm}>Form</button>
		<button type="button" class:active={mode === 'json'} onclick={toJSON}>JSON</button>
	</div>
	{#if formError}<div class="alert bad">{formError}</div>{/if}

	{#if mode === 'json'}
		<JsonEditor bind:value={json} bind:error={jsonError} rows={24} />
		<p class="hint" style="margin:0">Native StreamConfig JSON, as used by the nats CLI. Durations are nanoseconds. Use this for mirrors, sources, republish and subject transforms.</p>
	{:else}
		<div class="grid-2">
			<label class="field"
				><span>Name</span><input bind:value={name} required disabled={editing} pattern="[^.*> ]+" placeholder="ORDERS" /></label
			>
			<label class="field"><span>Description</span><input bind:value={description} /></label>
		</div>
		<label class="field">
			<span>Subjects</span>
			<textarea rows="3" bind:value={subjects} placeholder="orders.>&#10;returns.*"></textarea>
			<span class="hint">One per line; wildcards * and > allowed. Leave empty for mirrors and source-only streams.</span>
		</label>
		<div class="grid-3">
			<label class="field"
				><span>Storage</span><select bind:value={storage} disabled={editing}><option value="file">File</option><option value="memory">Memory</option></select
				></label
			>
			<label class="field"
				><span>Replicas</span><select bind:value={replicas}>{#each [1, 2, 3, 4, 5] as r}<option value={r}>{r}</option>{/each}</select></label
			>
			<label class="field">
				<span>Retention</span>
				<select bind:value={retention} disabled={editing}>
					<option value="limits">Limits (keep until limits)</option>
					<option value="interest">Interest (until all consumers ack)</option>
					<option value="workqueue">Work queue (until one consumer acks)</option>
				</select>
			</label>
		</div>
		<fieldset>
			<legend>Limits <span class="faint">(empty = unlimited)</span></legend>
			<div class="grid-3">
				<label class="field"><span>Max messages</span><input bind:value={maxMsgs} placeholder="unlimited" /></label>
				<label class="field"><span>Max size</span><input bind:value={maxBytes} placeholder="e.g. 10GiB" /></label>
				<label class="field"><span>Max age</span><input bind:value={maxAge} placeholder="e.g. 72h" /></label>
				<label class="field"><span>Max per subject</span><input bind:value={maxPerSubject} placeholder="unlimited" /></label>
				<label class="field"><span>Max message size</span><input bind:value={maxMsgSize} placeholder="e.g. 1MiB" /></label>
				<label class="field">
					<span>When full</span>
					<select bind:value={discard}><option value="old">Discard oldest</option><option value="new">Reject new messages</option></select>
				</label>
				<label class="field"><span>Duplicate window</span><input bind:value={dupWindow} placeholder="2m" /></label>
			</div>
		</fieldset>
		<fieldset>
			<legend>Placement</legend>
			<div class="grid-2">
				<label class="field">
					<span>Cluster</span>
					<select bind:value={cluster}>
						<option value="">(where the request lands)</option>
						{#each clusters as c}<option value={c}>{c}</option>{/each}
					</select>
					{#if editing}<span class="hint">Changing the cluster moves the stream.</span>{/if}
				</label>
				<label class="field"><span>Tags</span><input bind:value={tags} placeholder="az:1, ssd" /></label>
			</div>
		</fieldset>
		<fieldset>
			<legend>Options</legend>
			<div class="opts">
				<label class="check"><input type="checkbox" bind:checked={compression} />S2 compression</label>
				<label class="check"><input type="checkbox" bind:checked={allowDirect} />Allow direct get</label>
				<label class="check"><input type="checkbox" bind:checked={allowRollup} />Allow rollup headers</label>
				<label class="check"><input type="checkbox" bind:checked={allowTTL} />Allow per-message TTL</label>
				<label class="check"><input type="checkbox" bind:checked={denyDelete} />Deny message delete</label>
				<label class="check"><input type="checkbox" bind:checked={denyPurge} />Deny purge</label>
			</div>
		</fieldset>
	{/if}
	<div class="row">
		<button class="btn primary" disabled={busy || (mode === 'json' && !!jsonError)}>{busy ? 'Saving…' : submitText}</button>
	</div>
</form>

<style>
	fieldset {
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		padding: 10px 14px 14px;
		margin: 0;
	}
	legend {
		font-size: 12.5px;
		font-weight: 600;
		padding: 0 6px;
		color: var(--text-2);
	}
	.opts {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
		gap: 8px;
	}
</style>
