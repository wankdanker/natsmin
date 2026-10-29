<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api, unwrap, errMsg, type Schemas } from '$lib/api/client';
	import { can } from '$lib/session.svelte';
	import { toast } from '$lib/toast.svelte';
	import { getLive } from '$lib/context';
	import { bytes, num } from '$lib/format';
	import AccountHead from '$lib/AccountHead.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import Loading from '$lib/components/Loading.svelte';
	import Empty from '$lib/components/Empty.svelte';
	import Modal from '$lib/components/Modal.svelte';

	const path = $derived({ system: page.params.system!, account: page.params.account! });
	const live = getLive();
	let buckets = $state<Schemas['KVStatusOut'][] | null>(null);
	let error = $state<string | null>(null);
	let open = $state(false);
	let busy = $state(false);
	let form = $state<Schemas['KVConfigIn']>({ bucket: '', history: 1, storage: 'file', replicas: 1 });
	let cluster = $state('');

	async function load() {
		try {
			buckets = await unwrap(api.GET('/v1/systems/{system}/accounts/{account}/kv', { params: { path } }));
			error = null;
		} catch (e) {
			error = errMsg(e);
		}
	}
	onMount(() => {
		load();
		const t = setInterval(() => !document.hidden && !open && load(), 5000);
		return () => clearInterval(t);
	});

	async function create(e: SubmitEvent) {
		e.preventDefault();
		busy = true;
		try {
			const b = await unwrap(
				api.POST('/v1/systems/{system}/accounts/{account}/kv', {
					params: { path },
					body: { ...form, history: Number(form.history), replicas: Number(form.replicas), placement: cluster ? { cluster } : undefined }
				})
			);
			toast(`Created bucket ${b.bucket}`);
			open = false;
			goto(`/systems/${path.system}/a/${path.account}/kv/${encodeURIComponent(b.bucket)}`);
		} catch (err) {
			toast(errMsg(err), 'bad');
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>Key-Value · natsmin</title></svelte:head>

<div class="page" style="max-width:none">
	<AccountHead title="Key-Value buckets">
		{#snippet actions()}
			{#if can('operator')}<button class="btn primary" onclick={() => (open = true)}><Icon name="plus" />New bucket</button>{/if}
		{/snippet}
	</AccountHead>

	{#if error}<div class="alert bad">{error}</div>
	{:else if !buckets}<Loading />
	{:else if buckets.length === 0}
		<div class="card">
			<Empty icon="kv" title="No buckets yet">
				Key-Value buckets are streams with a key per subject, history and watch support.
				{#if can('operator')}<div style="margin-top:12px"><button class="btn primary" onclick={() => (open = true)}><Icon name="plus" />Create a bucket</button></div>{/if}
			</Empty>
		</div>
	{:else}
		<div class="card table-wrap">
			<table class="t">
				<thead><tr><th>Bucket</th><th class="r">Keys/values</th><th class="r">Size</th><th>History</th><th>TTL</th><th>Storage</th><th>Replicas</th></tr></thead>
				<tbody>
					{#each buckets as b (b.bucket)}
						<tr class="clickable" onclick={() => goto(`/systems/${path.system}/a/${path.account}/kv/${encodeURIComponent(b.bucket)}`)}>
							<td><b>{b.bucket}</b>{#if b.description}<div class="faint" style="font-size:12px">{b.description}</div>{/if}</td>
							<td class="r num">{num(b.values)}</td>
							<td class="r num">{bytes(b.bytes)}</td>
							<td>{b.history}</td>
							<td>{b.ttl || '—'}</td>
							<td>{b.storage}{b.compression ? ' · s2' : ''}</td>
							<td>R{b.replicas}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>

<Modal bind:open title="New Key-Value bucket" width={620}>
	<form id="kvform" class="stack" onsubmit={create}>
		<div class="grid-2">
			<label class="field"><span>Bucket name</span><input bind:value={form.bucket} required pattern="[a-zA-Z0-9_\-]+" /></label>
			<label class="field"><span>Description</span><input bind:value={form.description} /></label>
			<label class="field"><span>History per key</span><input type="number" min="1" max="64" bind:value={form.history} /></label>
			<label class="field"><span>TTL</span><input bind:value={form.ttl} placeholder="none, or e.g. 24h" /></label>
			<label class="field"><span>Storage</span><select bind:value={form.storage}><option value="file">File</option><option value="memory">Memory</option></select></label>
			<label class="field"><span>Replicas</span><select bind:value={form.replicas}>{#each [1, 2, 3, 4, 5] as r}<option value={r}>{r}</option>{/each}</select></label>
			<label class="field"><span>Max value size (bytes)</span><input type="number" bind:value={form.maxValueSize} placeholder="unlimited" /></label>
			<label class="field">
				<span>Cluster</span>
				<select bind:value={cluster}><option value="">(default)</option>{#each live.snapshot?.clusters ?? [] as c}<option value={c.name}>{c.name}</option>{/each}</select>
			</label>
		</div>
		<label class="check"><input type="checkbox" bind:checked={form.compression} />S2 compression</label>
	</form>
	{#snippet footer()}
		<button class="btn" onclick={() => (open = false)}>Cancel</button>
		<button class="btn primary" form="kvform" disabled={busy}>Create bucket</button>
	{/snippet}
</Modal>
