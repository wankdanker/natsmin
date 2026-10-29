<script lang="ts">
	import { onMount } from 'svelte';
	import { systems, refreshSystems } from '$lib/systems.svelte';
	import { can } from '$lib/session.svelte';
	import { rate, compact, ago } from '$lib/format';
	import Icon from '$lib/components/Icon.svelte';
	import Empty from '$lib/components/Empty.svelte';

	onMount(() => {
		const t = setInterval(() => refreshSystems().catch(() => {}), 5000);
		return () => clearInterval(t);
	});

	function state(s: (typeof systems.list)[number]) {
		if (!s.status.monitored) return { tone: '', text: 'Not monitored' };
		if (!s.status.connected || s.status.error) return { tone: 'bad', text: s.status.error ? 'Error' : 'Disconnected' };
		if (s.status.healthy < s.status.servers) return { tone: 'warn', text: 'Degraded' };
		return { tone: 'ok', text: 'Healthy' };
	}
</script>

<svelte:head><title>Systems · natsmin</title></svelte:head>

<div class="page">
	<div class="page-head">
		<div>
			<h1>Systems</h1>
			<div class="sub">Every NATS system natsmin manages</div>
		</div>
		<span class="spacer"></span>
		{#if can('admin')}<a href="/systems/new" class="btn primary"><Icon name="plus" />Add system</a>{/if}
	</div>

	{#if systems.list.length === 0}
		<div class="card">
			<Empty icon="topology" title="No systems yet">
				{#if can('admin')}
					Register a NATS server, cluster or supercluster to start monitoring it.
					<div style="margin-top:14px"><a href="/systems/new" class="btn primary"><Icon name="plus" />Add your first system</a></div>
				{:else}
					Ask an admin to register a NATS system.
				{/if}
			</Empty>
		</div>
	{:else}
		<div class="grid">
			{#each systems.list as s (s.id)}
				{@const st = state(s)}
				<a href="/systems/{s.id}" class="card sys">
					<div class="top">
						<div class="min">
							<h2 class="ellipsis">{s.name}</h2>
							<div class="faint ellipsis mono small">{s.urls.join(', ')}</div>
						</div>
						<span class="badge {st.tone}"><span class="dot {st.tone}"></span>{st.text}</span>
					</div>
					{#if s.description}<p class="muted desc">{s.description}</p>{/if}
					{#if s.status.monitored}
						<div class="metrics">
							<div><span class="faint">Servers</span><b class="num">{s.status.healthy}/{s.status.servers}</b></div>
							<div><span class="faint">Clusters</span><b class="num">{s.status.clusters}</b></div>
							<div><span class="faint">Connections</span><b class="num">{compact(s.status.connections)}</b></div>
							<div><span class="faint">Msgs in</span><b class="num">{rate(s.status.inMsgsRate)}</b></div>
						</div>
						{#if s.status.error}<div class="alert bad small">{s.status.error}</div>{/if}
					{:else}
						<p class="faint small">Add system account credentials in settings to see topology and stats.</p>
					{/if}
					<div class="foot faint small">
						{#each s.status.versions as v}<span class="badge">v{v}</span>{/each}
						{#if s.status.jetstream}<span class="badge accent">JetStream</span>{/if}
						<span class="badge">{s.accounts.length} account{s.accounts.length === 1 ? '' : 's'}</span>
						<span class="spacer"></span>
						{#if s.status.lastPoll}<span>polled {ago(s.status.lastPoll)}</span>{/if}
					</div>
				</a>
			{/each}
		</div>
	{/if}
</div>

<style>
	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(340px, 1fr));
		gap: 14px;
	}
	.sys {
		padding: 16px;
		color: var(--text);
		text-decoration: none;
		display: flex;
		flex-direction: column;
		gap: 12px;
		transition: border-color 0.15s;
	}
	.sys:hover {
		border-color: var(--accent);
		text-decoration: none;
	}
	.top {
		display: flex;
		align-items: flex-start;
		gap: 10px;
		justify-content: space-between;
	}
	.min {
		min-width: 0;
	}
	.small {
		font-size: 12px;
	}
	.desc {
		margin: 0;
		font-size: 13px;
	}
	.metrics {
		display: grid;
		grid-template-columns: repeat(4, 1fr);
		gap: 8px;
	}
	.metrics div {
		display: flex;
		flex-direction: column;
		font-size: 12px;
	}
	.metrics b {
		font-size: 17px;
		font-weight: 600;
	}
	.foot {
		display: flex;
		align-items: center;
		gap: 6px;
		flex-wrap: wrap;
		margin-top: auto;
	}
</style>
