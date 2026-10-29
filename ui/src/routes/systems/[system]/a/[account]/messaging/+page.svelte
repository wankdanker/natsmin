<script lang="ts">
	import { onDestroy } from 'svelte';
	import { page } from '$app/state';
	import { api, unwrap, errMsg, type Schemas } from '$lib/api/client';
	import { can } from '$lib/session.svelte';
	import { toast } from '$lib/toast.svelte';
	import { bytes, prettyJSON, hexdump } from '$lib/format';
	import AccountHead from '$lib/AccountHead.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import Empty from '$lib/components/Empty.svelte';

	type TailMsg = Schemas['TailMsg'];
	const path = $derived({ system: page.params.system!, account: page.params.account! });

	// Publish / request.
	let mode = $state<'publish' | 'request'>('publish');
	let subject = $state('');
	let payload = $state('');
	let headers = $state('');
	let useJS = $state(false);
	let timeout = $state(5000);
	let sending = $state(false);
	let reply = $state<{ msg: TailMsg; ms: number } | null>(null);
	let pubResult = $state<Schemas['PublishOut'] | null>(null);

	function parseHeaders(): Record<string, string[]> | undefined {
		const out: Record<string, string[]> = {};
		for (const line of headers.split('\n')) {
			const i = line.indexOf(':');
			if (i <= 0) continue;
			const k = line.slice(0, i).trim();
			(out[k] ??= []).push(line.slice(i + 1).trim());
		}
		return Object.keys(out).length ? out : undefined;
	}

	async function send(e: SubmitEvent) {
		e.preventDefault();
		sending = true;
		reply = null;
		pubResult = null;
		try {
			if (mode === 'publish') {
				pubResult = await unwrap(
					api.POST('/v1/systems/{system}/accounts/{account}/publish', { params: { path }, body: { subject, data: payload, headers: parseHeaders(), jetstream: useJS } })
				);
				toast(pubResult.stream ? `Stored in ${pubResult.stream} as #${pubResult.seq}` : `Published to ${subject}`);
			} else {
				const t0 = performance.now();
				const msg = await unwrap(
					api.POST('/v1/systems/{system}/accounts/{account}/request', { params: { path }, body: { subject, data: payload, headers: parseHeaders(), timeoutMs: timeout } })
				);
				reply = { msg, ms: performance.now() - t0 };
			}
		} catch (err) {
			toast(errMsg(err), 'bad');
		} finally {
			sending = false;
		}
	}

	// Live tail.
	let tailSubject = $state('>');
	let tailQueue = $state('');
	let tailing = $state(false);
	let paused = $state(false);
	let msgs = $state<(TailMsg & { id: number; at: number })[]>([]);
	let total = $state(0);
	let dropped = $state(0);
	let openId = $state<number | null>(null);
	let es: EventSource | null = null;
	let nextId = 0;

	function startTail(e?: SubmitEvent) {
		e?.preventDefault();
		stopTail();
		const q = new URLSearchParams({ subject: tailSubject });
		if (tailQueue) q.set('queue', tailQueue);
		es = new EventSource(`/api/v1/systems/${path.system}/accounts/${path.account}/tail?${q}`);
		es.addEventListener('message', (ev) => {
			total++;
			if (paused) return;
			const m = JSON.parse((ev as MessageEvent).data) as TailMsg;
			msgs = [{ ...m, id: nextId++, at: Date.now() }, ...msgs].slice(0, 500);
		});
		es.addEventListener('dropped', (ev) => {
			dropped += (JSON.parse((ev as MessageEvent).data) as { dropped: number }).dropped;
		});
		es.onerror = () => {
			if (es?.readyState === EventSource.CLOSED) {
				tailing = false;
				toast('Tail disconnected', 'bad');
			}
		};
		tailing = true;
		total = 0;
		dropped = 0;
	}
	function stopTail() {
		es?.close();
		es = null;
		tailing = false;
	}
	onDestroy(stopTail);

	function show(m: TailMsg) {
		if (m.text == null) return hexdump(m.data);
		return m.isJson ? prettyJSON(m.text) : m.text;
	}
</script>

<svelte:head><title>Publish & tail · natsmin</title></svelte:head>

<div class="page" style="max-width:none">
	<AccountHead title="Publish & tail" sub="Send messages, make requests and watch subjects live as this account." />

	{#if !can('operator')}
		<div class="alert info">Publishing and tailing need the operator role.</div>
	{:else}
		<div class="layout">
			<div class="card">
				<div class="card-head">
					<div class="tabs" style="margin:0;border:none">
						<button class:active={mode === 'publish'} onclick={() => (mode = 'publish')}>Publish</button>
						<button class:active={mode === 'request'} onclick={() => (mode = 'request')}>Request</button>
					</div>
				</div>
				<form class="card-body stack" onsubmit={send}>
					<label class="field"><span>Subject</span><input bind:value={subject} required class="mono" placeholder="orders.eu.created" /></label>
					<label class="field"><span>Payload</span><textarea rows="8" bind:value={payload} placeholder={'{"id": 42}'}></textarea></label>
					<label class="field"><span>Headers</span><textarea rows="2" bind:value={headers} placeholder="Nats-Msg-Id: abc-123"></textarea><span class="hint">One per line, Key: value.</span></label>
					{#if mode === 'publish'}
						<label class="check"><input type="checkbox" bind:checked={useJS} />Wait for a JetStream acknowledgement</label>
					{:else}
						<label class="field"><span>Timeout (ms)</span><input type="number" bind:value={timeout} min="1" max="60000" /></label>
					{/if}
					<div class="row">
						<button class="btn primary" disabled={sending}><Icon name="send" />{mode === 'publish' ? 'Publish' : 'Send request'}</button>
						{#if pubResult}
							<span class="faint">{pubResult.stream ? `stored in ${pubResult.stream} #${pubResult.seq}${pubResult.duplicate ? ' (duplicate)' : ''}` : 'published'}</span>
						{/if}
					</div>
					{#if reply}
						<div>
							<div class="row faint" style="font-size:12.5px;margin-bottom:6px">Reply in {reply.ms.toFixed(0)} ms · {bytes(reply.msg.size)}</div>
							{#if reply.msg.headers}
								<div class="mono faint" style="font-size:12px;margin-bottom:6px">
									{#each Object.entries(reply.msg.headers) as [k, v]}<div>{k}: {v.join(', ')}</div>{/each}
								</div>
							{/if}
							<pre class="payload">{show(reply.msg)}</pre>
						</div>
					{/if}
				</form>
			</div>

			<div class="card">
				<form class="card-head tailbar" onsubmit={startTail}>
					<input bind:value={tailSubject} class="mono" placeholder="Subject, e.g. orders.>" required />
					<input bind:value={tailQueue} placeholder="queue group (optional)" style="max-width:180px" />
					{#if tailing}
						<button type="button" class="btn" onclick={() => (paused = !paused)}><Icon name={paused ? 'play' : 'pause'} />{paused ? 'Resume' : 'Pause'}</button>
						<button type="button" class="btn danger" onclick={stopTail}><Icon name="stop" />Stop</button>
					{:else}
						<button class="btn primary"><Icon name="play" />Tail</button>
					{/if}
					<button type="button" class="btn ghost" onclick={() => ((msgs = []), (total = 0), (dropped = 0))}>Clear</button>
				</form>
				<div class="statusbar faint">
					{#if tailing}<span class="dot ok"></span> listening on <code>{tailSubject}</code>{:else}not listening{/if}
					<span class="spacer"></span>
					{total} received{#if dropped} · <span style="color:var(--warn)">{dropped} skipped (too fast)</span>{/if}
				</div>
				{#if msgs.length === 0}
					<Empty icon="activity" title={tailing ? 'Waiting for messages…' : 'Start a tail to watch a subject'} />
				{:else}
					<div class="tail">
						{#each msgs as m (m.id)}
							<button class="tm" class:open={openId === m.id} onclick={() => (openId = openId === m.id ? null : m.id)}>
								<span class="faint num">{new Date(m.at).toLocaleTimeString()}</span>
								<span class="mono subj">{m.subject}</span>
								<span class="mono faint prev">{m.text != null ? m.text.slice(0, 200) : `‹${bytes(m.size)} binary›`}</span>
								{#if m.reply}<span class="badge">reply</span>{/if}
							</button>
							{#if openId === m.id}
								<div class="tm-body">
									{#if m.reply}<div class="faint mono" style="font-size:12px">reply to: {m.reply}</div>{/if}
									{#if m.headers}{#each Object.entries(m.headers) as [k, v]}<div class="mono faint" style="font-size:12px">{k}: {v.join(', ')}</div>{/each}{/if}
									<pre class="payload">{show(m)}</pre>
								</div>
							{/if}
						{/each}
					</div>
				{/if}
			</div>
		</div>
	{/if}
</div>

<style>
	.layout {
		display: grid;
		grid-template-columns: minmax(320px, 0.8fr) minmax(0, 1.2fr);
		gap: 14px;
		align-items: start;
	}
	@media (max-width: 1100px) {
		.layout {
			grid-template-columns: 1fr;
		}
	}
	.tailbar {
		gap: 8px;
		flex-wrap: wrap;
	}
	.tailbar input:first-child {
		flex: 1;
		min-width: 160px;
	}
	.statusbar {
		display: flex;
		align-items: center;
		gap: 6px;
		font-size: 12px;
		padding: 6px 16px;
		border-bottom: 1px solid var(--border);
	}
	.tail {
		max-height: 70vh;
		overflow: auto;
	}
	.tm {
		display: grid;
		grid-template-columns: 80px minmax(100px, 0.5fr) 1fr auto;
		gap: 10px;
		width: 100%;
		text-align: left;
		padding: 6px 14px;
		border: none;
		border-bottom: 1px solid var(--border);
		background: var(--bg-elev);
		color: inherit;
		font: inherit;
		font-size: 12.5px;
		cursor: pointer;
		align-items: center;
	}
	.tm:hover,
	.tm.open {
		background: var(--bg-hover);
	}
	.subj,
	.prev {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.tm-body {
		padding: 8px 14px 12px;
		background: var(--bg-sunken);
		border-bottom: 1px solid var(--border);
	}
	.payload {
		background: var(--bg-sunken);
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		padding: 10px 12px;
		max-height: 360px;
		overflow: auto;
		margin-top: 6px;
	}
</style>
