<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api, unwrap, errMsg, type Schemas } from '$lib/api/client';
	import { can } from '$lib/session.svelte';
	import { toast } from '$lib/toast.svelte';
	import { confirm } from '$lib/components/Confirm.svelte';
	import { bytes, dateTime, ago, num, prettyJSON, hexdump } from '$lib/format';
	import AccountHead from '$lib/AccountHead.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import Loading from '$lib/components/Loading.svelte';
	import Empty from '$lib/components/Empty.svelte';

	type Entry = Schemas['KVEntryOut'];
	const path = $derived({ system: page.params.system!, account: page.params.account!, bucket: page.params.bucket! });
	let status = $state<Schemas['KVStatusOut'] | null>(null);
	let keys = $state<string[] | null>(null);
	let truncated = $state(false);
	let filter = $state('');
	let selected = $state<string | null>(null);
	let entry = $state<Entry | null>(null);
	let history = $state<Entry[]>([]);
	let editing = $state(false);
	let draft = $state('');
	let newKey = $state('');
	let creating = $state(false);
	let watching = $state(false);
	let events = $state<Entry[]>([]);
	let es: EventSource | null = null;
	let error = $state<string | null>(null);

	const shown = $derived((keys ?? []).filter((k) => !filter || k.toLowerCase().includes(filter.toLowerCase())));

	async function loadStatus() {
		status = await unwrap(api.GET('/v1/systems/{system}/accounts/{account}/kv/{bucket}', { params: { path } }));
	}
	async function loadKeys() {
		try {
			const r = await unwrap(api.GET('/v1/systems/{system}/accounts/{account}/kv/{bucket}/keys', { params: { path } }));
			keys = r.keys;
			truncated = r.truncated;
		} catch (e) {
			error = errMsg(e);
		}
	}
	async function select(k: string) {
		selected = k;
		editing = false;
		creating = false;
		entry = null;
		history = [];
		try {
			const [e, h] = await Promise.all([
				unwrap(api.GET('/v1/systems/{system}/accounts/{account}/kv/{bucket}/entry', { params: { path, query: { key: k } } })),
				unwrap(api.GET('/v1/systems/{system}/accounts/{account}/kv/{bucket}/history', { params: { path, query: { key: k } } }))
			]);
			entry = e;
			history = h;
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}

	onMount(() => {
		loadStatus().catch((e) => (error = errMsg(e)));
		loadKeys();
		return () => es?.close();
	});

	function toggleWatch() {
		if (watching) {
			es?.close();
			es = null;
			watching = false;
			return;
		}
		const p = path;
		es = new EventSource(`/api/v1/systems/${p.system}/accounts/${p.account}/kv/${encodeURIComponent(p.bucket)}/watch`);
		es.addEventListener('entry', (ev) => {
			const e = JSON.parse((ev as MessageEvent).data) as Entry;
			events = [e, ...events].slice(0, 200);
			if (keys) {
				if (e.operation === 'put' && !keys.includes(e.key)) keys = [...keys, e.key].sort();
				if (e.operation !== 'put') keys = keys.filter((k) => k !== e.key);
			}
			if (e.key === selected) {
				if (e.operation === 'put') {
					entry = e;
					history = [e, ...history];
				} else entry = null;
			}
		});
		es.onerror = () => {
			watching = false;
			es?.close();
		};
		watching = true;
	}

	function display(e: Entry) {
		if (e.text == null) return hexdump(e.value);
		return e.isJson ? prettyJSON(e.text) : e.text;
	}

	async function save(key: string, value: string, expect?: number) {
		try {
			const r = await unwrap(
				api.PUT('/v1/systems/{system}/accounts/{account}/kv/{bucket}/entry', { params: { path }, body: { key, value, expectRevision: expect } })
			);
			toast(`Saved ${key} at revision ${r.revision}`);
			if (keys && !keys.includes(key)) keys = [...keys, key].sort();
			await select(key);
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}

	async function del(purge: boolean) {
		if (!selected) return;
		const k = selected;
		if (!(await confirm({ title: purge ? 'Purge key' : 'Delete key', message: purge ? `Remove ${k} and all of its history?` : `Delete ${k}? Its history is kept.`, confirmText: purge ? 'Purge' : 'Delete', danger: true })))
			return;
		try {
			await unwrap(api.DELETE('/v1/systems/{system}/accounts/{account}/kv/{bucket}/entry', { params: { path, query: { key: k, purge } } }));
			toast(`${purge ? 'Purged' : 'Deleted'} ${k}`);
			keys = keys?.filter((x) => x !== k) ?? null;
			selected = null;
			entry = null;
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}

	async function deleteBucket() {
		if (!(await confirm({ title: 'Delete bucket', message: `Delete bucket ${path.bucket} and every key in it?`, confirmText: 'Delete bucket', danger: true, typeToConfirm: path.bucket })))
			return;
		try {
			await unwrap(api.DELETE('/v1/systems/{system}/accounts/{account}/kv/{bucket}', { params: { path } }));
			toast(`Deleted ${path.bucket}`);
			goto(`/systems/${path.system}/a/${path.account}/kv`);
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}
</script>

<svelte:head><title>{path.bucket} · Key-Value · natsmin</title></svelte:head>

<div class="page" style="max-width:none">
	<AccountHead
		title={path.bucket}
		sub={status ? `${num(status.values)} values · ${bytes(status.bytes)} · history ${status.history}${status.ttl ? ` · TTL ${status.ttl}` : ''} · R${status.replicas} ${status.storage}` : undefined}
		crumbs={[['Key-Value', `/systems/${path.system}/a/${path.account}/kv`]]}
	>
		{#snippet actions()}
			<button class="btn" class:primary={watching} onclick={toggleWatch}><Icon name={watching ? 'stop' : 'eye'} />{watching ? 'Stop watching' : 'Watch'}</button>
			{#if can('operator')}
				<button class="btn" onclick={() => ((creating = true), (selected = null), (newKey = ''), (draft = ''))}><Icon name="plus" />New key</button>
				<button class="btn danger" onclick={deleteBucket}><Icon name="trash" />Delete bucket</button>
			{/if}
		{/snippet}
	</AccountHead>

	{#if error}<div class="alert bad">{error}</div>{/if}

	<div class="kvl">
		<div class="card keys">
			<div class="card-head">
				<input type="search" placeholder="Filter keys…" bind:value={filter} />
				<button class="btn sm ghost icon" onclick={loadKeys} title="Reload" aria-label="Reload"><Icon name="refresh" size={14} /></button>
			</div>
			{#if !keys}<Loading />
			{:else if keys.length === 0}<Empty icon="kv" title="No keys" />
			{:else}
				<div class="list">
					{#each shown.slice(0, 5000) as k (k)}
						<button class="key mono" class:sel={k === selected} onclick={() => select(k)}>{k}</button>
					{/each}
				</div>
				<div class="faint count">{num(shown.length)} key{shown.length === 1 ? '' : 's'}{truncated ? ' (truncated)' : ''}</div>
			{/if}
		</div>

		<div class="stack">
			{#if creating}
				<div class="card">
					<div class="card-head"><h2>New key</h2></div>
					<div class="card-body stack">
						<label class="field"><span>Key</span><input bind:value={newKey} placeholder="app.config.mode" class="mono" /></label>
						<label class="field"><span>Value</span><textarea rows="10" bind:value={draft}></textarea></label>
						<div class="row">
							<button class="btn primary" disabled={!newKey} onclick={() => save(newKey, draft, 0)}>Create</button>
							<button class="btn" onclick={() => (creating = false)}>Cancel</button>
						</div>
					</div>
				</div>
			{:else if selected}
				<div class="card">
					<div class="card-head">
						<h2 class="mono ellipsis">{selected}</h2>
						<span class="spacer"></span>
						{#if entry && can('operator')}
							{#if !editing}<button class="btn sm" onclick={() => ((draft = entry?.text ?? ''), (editing = true))} disabled={entry.text == null}><Icon name="edit" size={13} />Edit</button>{/if}
							<button class="btn sm danger" onclick={() => del(false)}>Delete</button>
							<button class="btn sm danger" onclick={() => del(true)}>Purge</button>
						{/if}
					</div>
					<div class="card-body">
						{#if !entry}<Loading />
						{:else}
							<div class="row faint" style="font-size:12.5px;margin-bottom:8px">
								<span>revision {entry.revision}</span>·<span>{bytes(entry.size)}</span>·<span title={dateTime(entry.created)}>{ago(entry.created)}</span>
								{#if entry.isJson}<span class="badge">JSON</span>{/if}
							</div>
							{#if editing}
								<textarea rows="14" bind:value={draft}></textarea>
								<div class="row" style="margin-top:8px">
									<button class="btn primary" onclick={() => save(selected!, draft, entry!.revision)}>Save revision</button>
									<button class="btn" onclick={() => (editing = false)}>Cancel</button>
									<span class="faint" style="font-size:12px">Saves only if nobody changed the key since revision {entry.revision}.</span>
								</div>
							{:else}
								<pre class="val">{display(entry)}</pre>
							{/if}
						{/if}
					</div>
				</div>
				{#if history.length > 1}
					<div class="card">
						<div class="card-head"><h2>History</h2><span class="badge">{history.length}</span></div>
						<table class="t">
							<thead><tr><th>Revision</th><th>Operation</th><th>When</th><th>Value</th>{#if can('operator')}<th></th>{/if}</tr></thead>
							<tbody>
								{#each history as h (h.revision)}
									<tr>
										<td class="num">{h.revision}</td>
										<td><span class="badge {h.operation === 'put' ? '' : 'warn'}">{h.operation}</span></td>
										<td class="nowrap">{dateTime(h.created)}</td>
										<td class="mono ellipsis" style="max-width:420px">{h.text ?? `‹${bytes(h.size)} binary›`}</td>
										{#if can('operator')}
											<td>{#if h.operation === 'put' && h.revision !== entry?.revision && h.text != null}<button class="btn sm" onclick={() => save(selected!, h.text!)}>Restore</button>{/if}</td>
										{/if}
									</tr>
								{/each}
							</tbody>
						</table>
					</div>
				{/if}
			{:else}
				<div class="card"><Empty icon="kv" title="Select a key">Pick a key on the left to see its value and history.</Empty></div>
			{/if}

			{#if watching}
				<div class="card">
					<div class="card-head"><span class="dot ok"></span><h2>Live changes</h2><span class="spacer"></span><button class="btn sm" onclick={() => (events = [])}>Clear</button></div>
					{#if events.length === 0}<div class="card-body faint">Waiting for changes…</div>
					{:else}
						<table class="t">
							<tbody>
								{#each events as e (e.key + e.revision)}
									<tr>
										<td class="nowrap faint">{new Date(e.created).toLocaleTimeString()}</td>
										<td><span class="badge {e.operation === 'put' ? 'ok' : 'warn'}">{e.operation}</span></td>
										<td class="mono"><button class="link" onclick={() => select(e.key)}>{e.key}</button></td>
										<td class="mono ellipsis" style="max-width:360px">{e.text ?? ''}</td>
									</tr>
								{/each}
							</tbody>
						</table>
					{/if}
				</div>
			{/if}
		</div>
	</div>
</div>

<style>
	.kvl {
		display: grid;
		grid-template-columns: 320px minmax(0, 1fr);
		gap: 14px;
		align-items: start;
	}
	@media (max-width: 900px) {
		.kvl {
			grid-template-columns: 1fr;
		}
	}
	.keys .list {
		max-height: 65vh;
		overflow: auto;
		padding: 4px;
	}
	.key {
		display: block;
		width: 100%;
		text-align: left;
		background: none;
		border: none;
		padding: 5px 10px;
		border-radius: 4px;
		color: var(--text);
		cursor: pointer;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.key:hover {
		background: var(--bg-hover);
	}
	.key.sel {
		background: var(--accent-soft);
		color: var(--accent);
	}
	.count {
		font-size: 12px;
		padding: 8px 12px;
		border-top: 1px solid var(--border);
	}
	.val {
		background: var(--bg-sunken);
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		padding: 12px;
		max-height: 480px;
		overflow: auto;
	}
	.link {
		background: none;
		border: none;
		padding: 0;
		color: var(--accent);
		cursor: pointer;
		font: inherit;
	}
</style>
