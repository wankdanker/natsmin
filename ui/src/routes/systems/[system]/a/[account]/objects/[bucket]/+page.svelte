<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api, unwrap, errMsg, rawFetch, type Schemas } from '$lib/api/client';
	import { can } from '$lib/session.svelte';
	import { toast } from '$lib/toast.svelte';
	import { confirm } from '$lib/components/Confirm.svelte';
	import { bytes, dateTime } from '$lib/format';
	import AccountHead from '$lib/AccountHead.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import Loading from '$lib/components/Loading.svelte';
	import Empty from '$lib/components/Empty.svelte';

	type Obj = Schemas['ObjInfoOut'];
	const path = $derived({ system: page.params.system!, account: page.params.account!, bucket: page.params.bucket! });
	const apiBase = $derived(`/v1/systems/${path.system}/accounts/${path.account}/objects/${encodeURIComponent(path.bucket)}`);
	let data = $state<{ status: Schemas['ObjStatusOut']; objects: Obj[] } | null>(null);
	let error = $state<string | null>(null);
	let filter = $state('');
	let uploading = $state<string | null>(null);
	let dragOver = $state(false);
	let input: HTMLInputElement | undefined = $state();
	const shown = $derived((data?.objects ?? []).filter((o) => !filter || o.name.toLowerCase().includes(filter.toLowerCase())));

	async function load() {
		try {
			data = await unwrap(api.GET('/v1/systems/{system}/accounts/{account}/objects/{bucket}', { params: { path } }));
			error = null;
		} catch (e) {
			error = errMsg(e);
		}
	}
	onMount(load);

	async function upload(files: FileList | null) {
		if (!files) return;
		for (const f of Array.from(files)) {
			uploading = f.name;
			try {
				await rawFetch(`${apiBase}/upload?name=${encodeURIComponent(f.name)}`, {
					method: 'PUT',
					headers: { 'Content-Type': 'application/octet-stream' },
					body: f
				});
				toast(`Uploaded ${f.name}`);
			} catch (e) {
				toast(`${f.name}: ${errMsg(e)}`, 'bad');
			}
		}
		uploading = null;
		load();
	}

	async function del(o: Obj) {
		if (!(await confirm({ title: 'Delete object', message: `Delete ${o.name} (${bytes(o.size)})?`, confirmText: 'Delete', danger: true }))) return;
		try {
			await unwrap(api.DELETE('/v1/systems/{system}/accounts/{account}/objects/{bucket}/object', { params: { path, query: { name: o.name } } }));
			toast(`Deleted ${o.name}`);
			load();
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}

	async function deleteBucket() {
		if (!(await confirm({ title: 'Delete bucket', message: `Delete ${path.bucket} and every object in it?`, confirmText: 'Delete bucket', danger: true, typeToConfirm: path.bucket }))) return;
		try {
			await unwrap(api.DELETE('/v1/systems/{system}/accounts/{account}/objects/{bucket}', { params: { path } }));
			toast(`Deleted ${path.bucket}`);
			goto(`/systems/${path.system}/a/${path.account}/objects`);
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}
</script>

<svelte:head><title>{path.bucket} · Object store · natsmin</title></svelte:head>

<div class="page" style="max-width:none">
	<AccountHead
		title={path.bucket}
		sub={data ? `${data.objects.length} objects · ${bytes(data.status.size)} · R${data.status.replicas} ${data.status.storage}` : undefined}
		crumbs={[['Object store', `/systems/${path.system}/a/${path.account}/objects`]]}
	>
		{#snippet actions()}
			<input type="search" placeholder="Filter…" bind:value={filter} style="width:200px" />
			{#if can('operator')}
				<input type="file" multiple bind:this={input} onchange={(e) => upload((e.currentTarget as HTMLInputElement).files)} hidden />
				<button class="btn primary" onclick={() => input?.click()} disabled={!!uploading}><Icon name="upload" />{uploading ? `Uploading ${uploading}…` : 'Upload'}</button>
				<button class="btn danger" onclick={deleteBucket}><Icon name="trash" />Delete bucket</button>
			{/if}
		{/snippet}
	</AccountHead>

	{#if error}<div class="alert bad">{error}</div>
	{:else if !data}<Loading />
	{:else}
		<div
			class="card table-wrap drop"
			class:over={dragOver}
			role="region"
			aria-label="Objects"
			ondragover={(e) => {
				if (!can('operator')) return;
				e.preventDefault();
				dragOver = true;
			}}
			ondragleave={() => (dragOver = false)}
			ondrop={(e) => {
				e.preventDefault();
				dragOver = false;
				if (can('operator')) upload(e.dataTransfer?.files ?? null);
			}}
		>
			{#if shown.length === 0}
				<Empty icon="box" title="No objects">{can('operator') ? 'Drop files here or use Upload.' : ''}</Empty>
			{:else}
				<table class="t">
					<thead><tr><th>Name</th><th class="r">Size</th><th class="r">Chunks</th><th>Modified</th><th>Digest</th><th></th></tr></thead>
					<tbody>
						{#each shown as o (o.name)}
							<tr>
								<td class="mono"><b>{o.name}</b>{#if o.link}<span class="badge info">→ {o.link}</span>{/if}{#if o.description}<div class="faint">{o.description}</div>{/if}</td>
								<td class="r num">{bytes(o.size)}</td>
								<td class="r num">{o.chunks}</td>
								<td class="nowrap">{dateTime(o.modified)}</td>
								<td class="mono faint ellipsis" style="max-width:200px;font-size:11.5px" title={o.digest}>{o.digest}</td>
								<td class="nowrap r">
									<a class="btn sm" href="/api{apiBase}/download?name={encodeURIComponent(o.name)}" download><Icon name="download" size={13} /></a>
									{#if can('operator')}<button class="btn sm danger" onclick={() => del(o)} aria-label="Delete"><Icon name="trash" size={13} /></button>{/if}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			{/if}
		</div>
	{/if}
</div>

<style>
	.drop.over {
		outline: 2px dashed var(--accent);
		outline-offset: -6px;
	}
</style>
