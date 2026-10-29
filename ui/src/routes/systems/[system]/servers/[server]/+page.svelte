<script lang="ts">
	import { page } from '$app/state';
	import { getLive } from '$lib/context';
	import { api, unwrap, errMsg, type Schemas } from '$lib/api/client';
	import { can } from '$lib/session.svelte';
	import { toast } from '$lib/toast.svelte';
	import { confirm } from '$lib/components/Confirm.svelte';
	import { bytes, compact, dateTime, num, rate, bytesRate } from '$lib/format';
	import Icon from '$lib/components/Icon.svelte';
	import JsonView from '$lib/components/JsonView.svelte';
	import Loading from '$lib/components/Loading.svelte';
	import Meter from '$lib/components/Meter.svelte';

	type Kind = 'varz' | 'jsz' | 'healthz' | 'routez' | 'gatewayz' | 'leafz' | 'accountz' | 'raftz' | 'subsz' | 'ipqueuesz';
	const live = getLive();
	const id = $derived(page.params.server!);
	const server = $derived(live.snapshot?.servers.find((s) => s.id === id));
	let kind = $state<Kind>('varz');
	let raw = $state<Schemas['MonitorResponse'] | null>(null);
	let loading = $state(false);
	let error = $state<string | null>(null);

	async function load() {
		loading = true;
		error = null;
		try {
			const res = await unwrap(
				api.GET('/v1/systems/{system}/monitor/{kind}', {
					params: {
						path: { system: live.id, kind },
						query: { server: id, options: kind === 'jsz' ? '{"accounts":true,"streams":true}' : kind === 'subsz' ? '{"subscriptions":true,"limit":200}' : undefined }
					}
				})
			);
			raw = res[0] ?? null;
		} catch (e) {
			error = errMsg(e);
			raw = null;
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		kind;
		id;
		load();
	});

	async function reload() {
		if (!(await confirm({ title: 'Reload configuration', message: `Ask ${server?.name ?? id} to reload its configuration file?`, confirmText: 'Reload' }))) return;
		try {
			await unwrap(api.POST('/v1/systems/{system}/servers/{server}/reload', { params: { path: { system: live.id, server: id } } }));
			toast('Configuration reloaded');
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}

	async function stepdownMeta() {
		if (!(await confirm({ title: 'Move meta leadership here', message: `Ask the JetStream meta leader to step down in favour of ${server?.name}?`, confirmText: 'Step down' }))) return;
		try {
			await unwrap(api.POST('/v1/systems/{system}/jetstream/meta/stepdown', { params: { path: { system: live.id } }, body: { preferred: server?.name } }));
			toast('Meta leader stepped down');
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}
</script>

<svelte:head><title>{server?.name ?? id} · natsmin</title></svelte:head>

<div class="page">
	<div class="page-head">
		<div>
			<div class="faint" style="font-size:12.5px"><a href="/systems/{live.id}/servers">Servers</a> /</div>
			<h1 class="row" style="gap:8px">
				{#if server}<span class="dot {server.missing ? '' : server.healthy ? 'ok' : 'bad'}"></span>{/if}
				{server?.name ?? id}
			</h1>
			<div class="sub mono">{id}</div>
		</div>
		<span class="spacer"></span>
		<a class="btn" href="/systems/{live.id}/connections?server={encodeURIComponent(server?.name ?? '')}"><Icon name="plug" />Connections</a>
		{#if can('admin') && server?.jetstream && !server.jetstream.metaLeader}
			<button class="btn" onclick={stepdownMeta}><Icon name="crown" />Make meta leader</button>
		{/if}
		{#if can('admin')}<button class="btn" onclick={reload}><Icon name="refresh" />Reload config</button>{/if}
	</div>

	{#if server}
		{#if !server.healthy}
			<div class="alert bad" style="margin-bottom:14px">{server.healthError || 'The server reports itself unhealthy.'}</div>
		{/if}
		<div class="grid-3">
			<div class="card">
				<div class="card-head"><h2>Server</h2></div>
				<div class="card-body">
					<dl class="kv-list">
						<dt>Cluster</dt><dd>{server.cluster || '—'}</dd>
						<dt>Domain</dt><dd>{server.domain || '—'}</dd>
						<dt>Version</dt><dd>{server.version}</dd>
						<dt>Listen</dt><dd class="mono">{server.host}:{server.port}</dd>
						<dt>Started</dt><dd>{dateTime(server.start)} ({server.uptime})</dd>
						<dt>Config loaded</dt><dd>{dateTime(server.configLoadTime)}</dd>
						<dt>Tags</dt><dd>{#each server.tags ?? [] as t}<span class="badge">{t}</span> {:else}—{/each}</dd>
					</dl>
				</div>
			</div>
			<div class="card">
				<div class="card-head"><h2>Load</h2></div>
				<div class="card-body">
					<dl class="kv-list">
						<dt>CPU</dt><dd>{server.cpu.toFixed(1)}% of {server.cores} cores</dd>
						<dt>Memory</dt><dd>{bytes(server.mem)}</dd>
						<dt>Connections</dt><dd>{num(server.connections)}{server.maxConnections ? ` / ${num(server.maxConnections)}` : ''} ({num(server.totalConnections)} total)</dd>
						<dt>Subscriptions</dt><dd>{num(server.subscriptions)}</dd>
						<dt>Messages in</dt><dd>{rate(server.inMsgsRate)} · {bytesRate(server.inBytesRate)}</dd>
						<dt>Messages out</dt><dd>{rate(server.outMsgsRate)} · {bytesRate(server.outBytesRate)}</dd>
						<dt>Slow consumers</dt><dd>{num(server.slowConsumers)}</dd>
						<dt>Links</dt><dd>{server.routes} routes · {server.gateways} gateways · {server.leafnodes} leafnodes</dd>
					</dl>
				</div>
			</div>
			<div class="card">
				<div class="card-head"><h2>JetStream</h2>{#if server.jetstream?.metaLeader}<span class="badge accent">meta leader</span>{/if}</div>
				<div class="card-body">
					{#if server.jetstream}
						{@const j = server.jetstream}
						<dl class="kv-list">
							<dt>Streams</dt><dd>{j.streams} ({j.streamsLeader} led)</dd>
							<dt>Consumers</dt><dd>{j.consumers} ({j.consumersLeader} led)</dd>
							<dt>Messages</dt><dd>{compact(j.messages)} · {bytes(j.bytes)}</dd>
							<dt>HA assets</dt><dd>{j.haAssets}</dd>
							<dt>API</dt><dd>{compact(j.apiTotal)} calls · {num(j.apiErrors)} errors · level {j.apiLevel}</dd>
						</dl>
						<div style="margin-top:12px;font-size:12px" class="row"><span class="muted">File</span><span class="spacer"></span>{bytes(j.storage)} / {bytes(j.maxStorage)}</div>
						<Meter used={j.storage} total={j.maxStorage} />
						<div style="margin-top:8px;font-size:12px" class="row"><span class="muted">Memory</span><span class="spacer"></span>{bytes(j.memory)} / {bytes(j.maxMemory)}</div>
						<Meter used={j.memory} total={j.maxMemory} />
					{:else}
						<span class="faint">JetStream is not enabled on this server.</span>
					{/if}
				</div>
			</div>
		</div>
	{/if}

	<div class="card" style="margin-top:14px">
		<div class="card-head">
			<h2>Raw monitoring</h2>
			<span class="spacer"></span>
			<button class="btn sm" onclick={load} disabled={loading}><Icon name="refresh" size={14} />Refresh</button>
		</div>
		<div class="card-body">
			<div class="tabs">
				{#each ['varz', 'jsz', 'healthz', 'routez', 'gatewayz', 'leafz', 'accountz', 'subsz', 'raftz', 'ipqueuesz'] as k}
					<button class:active={kind === k} onclick={() => (kind = k as Kind)}>{k}</button>
				{/each}
			</div>
			{#if loading && !raw}<Loading />{:else if error}<div class="alert bad">{error}</div>
			{:else if raw}
				{#if raw.error}<div class="alert bad">{raw.error.description}</div>{/if}
				<JsonView value={raw.data ?? null} maxHeight={600} />
			{/if}
		</div>
	</div>
</div>
