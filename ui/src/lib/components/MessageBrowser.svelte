<script lang="ts">
	import { untrack } from 'svelte';
	import { api, unwrap, errMsg, type Schemas } from '$lib/api/client';
	import { can } from '$lib/session.svelte';
	import { toast } from '$lib/toast.svelte';
	import { confirm } from './Confirm.svelte';
	import { bytes, dateTime, hexdump, prettyJSON } from '$lib/format';
	import Icon from './Icon.svelte';
	import CopyButton from './CopyButton.svelte';
	import Empty from './Empty.svelte';

	type Msg = Schemas['MsgOut'];
	let {
		system,
		account,
		stream,
		firstSeq,
		lastSeq,
		subject = $bindable('')
	}: { system: string; account: string; stream: string; firstSeq: number; lastSeq: number; subject?: string } = $props();

	let startSeq = $state('');
	let startTime = $state('');
	let limit = $state(50);
	let msgs = $state<Msg[]>([]);
	let nextSeq = $state(0);
	let loading = $state(false);
	let error = $state<string | null>(null);
	let open = $state<number | null>(null);
	let view = $state<'text' | 'hex'>('text');
	const path = $derived({ system, account, stream });

	async function fetchPage(append: boolean, from?: number) {
		loading = true;
		error = null;
		try {
			const query: Record<string, unknown> = { limit, subject: subject || undefined };
			if (from) query.startSeq = from;
			else if (startTime) query.startTime = new Date(startTime).toISOString();
			else if (startSeq) query.startSeq = parseInt(startSeq, 10);
			const page = await unwrap(
				api.GET('/v1/systems/{system}/accounts/{account}/jetstream/streams/{stream}/messages', { params: { path, query: query as never } })
			);
			msgs = append ? [...msgs, ...page.messages] : page.messages;
			nextSeq = page.nextSeq;
		} catch (e) {
			error = errMsg(e);
		} finally {
			loading = false;
		}
	}

	function newest() {
		startTime = '';
		startSeq = String(Math.max(firstSeq, lastSeq - limit + 1));
		fetchPage(false);
	}
	function oldest() {
		startTime = '';
		startSeq = '';
		fetchPage(false);
	}
	function search(e: SubmitEvent) {
		e.preventDefault();
		fetchPage(false);
	}

	$effect(() => {
		stream;
		untrack(() => (lastSeq > 0 ? newest() : (msgs = [])));
	});

	async function republish(m: Msg) {
		const target = prompt('Publish this message again to subject:', m.subject);
		if (!target) return;
		try {
			const r = await unwrap(
				api.POST('/v1/systems/{system}/accounts/{account}/jetstream/streams/{stream}/messages/{seq}/republish', {
					params: { path: { ...path, seq: m.seq } },
					body: { subject: target }
				})
			);
			toast(r.stream ? `Stored as ${r.stream} #${r.seq}` : `Published to ${target}`);
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}

	async function del(m: Msg) {
		if (!(await confirm({ title: 'Delete message', message: `Delete message #${m.seq} (${m.subject}) from ${stream}?`, confirmText: 'Delete', danger: true }))) return;
		try {
			await unwrap(
				api.DELETE('/v1/systems/{system}/accounts/{account}/jetstream/streams/{stream}/messages/{seq}', { params: { path: { ...path, seq: m.seq } } })
			);
			msgs = msgs.filter((x) => x.seq !== m.seq);
			toast(`Deleted #${m.seq}`);
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}

	function preview(m: Msg) {
		if (m.text == null) return `‹${bytes(m.size)} binary›`;
		return m.text.length > 160 ? m.text.slice(0, 160) + '…' : m.text;
	}
</script>

<form class="controls" onsubmit={search}>
	<label class="field"><span>Subject filter</span><input bind:value={subject} placeholder="> (all)" /></label>
	<label class="field"><span>From sequence</span><input bind:value={startSeq} placeholder={String(firstSeq)} oninput={() => (startTime = '')} /></label>
	<label class="field"><span>From time</span><input type="datetime-local" bind:value={startTime} step="1" /></label>
	<label class="field"
		><span>Page size</span><select bind:value={limit}>{#each [20, 50, 100, 250, 500] as n}<option value={n}>{n}</option>{/each}</select></label
	>
	<div class="field"><span>&nbsp;</span><button class="btn primary" disabled={loading}><Icon name="search" />Read</button></div>
	<div class="field"><span>&nbsp;</span><div class="row" style="flex-wrap:nowrap"><button type="button" class="btn" onclick={oldest}>Oldest</button><button type="button" class="btn" onclick={newest}>Newest</button></div></div>
</form>

{#if error}<div class="alert bad">{error}</div>{/if}

{#if msgs.length === 0 && !loading}
	<Empty icon="stream" title="No messages">{lastSeq === 0 ? 'This stream is empty.' : 'Nothing matches from this position.'}</Empty>
{:else}
	<div class="msgs">
		{#each msgs as m (m.seq)}
			<div class="msg" class:open={open === m.seq}>
				<button class="head" onclick={() => (open = open === m.seq ? null : m.seq)}>
					<span class="seq num">#{m.seq}</span>
					<span class="subj mono">{m.subject}</span>
					<span class="prev mono faint">{preview(m)}</span>
					<span class="meta faint nowrap">{bytes(m.size)} · {dateTime(m.time)}</span>
				</button>
				{#if open === m.seq}
					<div class="body">
						{#if m.headers && Object.keys(m.headers).length}
							<table class="t hdrs">
								<tbody>
									{#each Object.entries(m.headers) as [k, vs]}
										<tr><td class="mono">{k}</td><td class="mono">{vs.join(', ')}</td></tr>
									{/each}
								</tbody>
							</table>
						{/if}
						<div class="row" style="margin-bottom:6px">
							<div class="tabs" style="margin:0;border:none">
								<button class:active={view === 'text'} onclick={() => (view = 'text')}>{m.isJson ? 'JSON' : 'Text'}</button>
								<button class:active={view === 'hex'} onclick={() => (view = 'hex')}>Hex</button>
							</div>
							<span class="spacer"></span>
							<CopyButton text={m.text ?? m.data} label="Copy" />
							{#if can('operator')}
								<button class="btn sm" onclick={() => republish(m)}><Icon name="send" size={13} />Republish</button>
								<button class="btn sm danger" onclick={() => del(m)}><Icon name="trash" size={13} />Delete</button>
							{/if}
						</div>
						<pre class="payload">{view === 'hex' || m.text == null ? hexdump(m.data) : m.isJson ? prettyJSON(m.text) : m.text}</pre>
					</div>
				{/if}
			</div>
		{/each}
	</div>
	<div class="row" style="margin-top:10px">
		{#if nextSeq > 0}
			<button class="btn" disabled={loading} onclick={() => fetchPage(true, nextSeq)}>{loading ? 'Loading…' : 'Load more'}</button>
		{:else if msgs.length}
			<span class="faint">End of stream.</span>
		{/if}
	</div>
{/if}

<style>
	.controls {
		display: grid;
		grid-template-columns: 2fr 1fr 1.4fr 0.8fr auto auto;
		gap: 10px;
		margin-bottom: 12px;
		align-items: end;
	}
	@media (max-width: 1000px) {
		.controls {
			grid-template-columns: 1fr 1fr;
		}
	}
	.msgs {
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		overflow: hidden;
	}
	.msg + .msg {
		border-top: 1px solid var(--border);
	}
	.head {
		display: grid;
		grid-template-columns: 70px minmax(120px, 0.6fr) 1fr auto;
		gap: 12px;
		width: 100%;
		text-align: left;
		padding: 8px 12px;
		background: var(--bg-elev);
		border: none;
		color: inherit;
		font: inherit;
		cursor: pointer;
		align-items: center;
	}
	.head:hover,
	.msg.open .head {
		background: var(--bg-hover);
	}
	.seq {
		font-weight: 600;
		color: var(--accent);
	}
	.subj,
	.prev {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.meta {
		font-size: 12px;
	}
	.body {
		padding: 10px 14px 14px;
		background: var(--bg-sunken);
	}
	.hdrs {
		margin-bottom: 10px;
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
	}
	.hdrs td {
		padding: 4px 10px;
		font-size: 12px;
	}
	.payload {
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		padding: 10px 12px;
		max-height: 420px;
		overflow: auto;
	}
</style>
