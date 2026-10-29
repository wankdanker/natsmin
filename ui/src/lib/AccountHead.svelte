<script lang="ts">
	import type { Snippet } from 'svelte';
	import { page } from '$app/state';
	import { findSystem } from '$lib/systems.svelte';

	let { title, sub, crumbs = [], actions }: { title: string; sub?: string; crumbs?: [string, string][]; actions?: Snippet } = $props();
	const sys = $derived(findSystem(page.params.system!));
	const acc = $derived(sys?.accounts.find((a) => a.id === page.params.account));
</script>

<div class="page-head">
	<div class="min">
		<div class="faint crumbs">
			<a href="/systems/{page.params.system}">{sys?.name ?? page.params.system}</a> /
			<span>{acc?.name ?? page.params.account}</span>
			{#each crumbs as [label, href]} / <a {href}>{label}</a>{/each}
		</div>
		<h1 class="ellipsis">{title}</h1>
		{#if sub}<div class="sub">{sub}</div>{/if}
	</div>
	<span class="spacer"></span>
	{#if actions}<div class="row">{@render actions()}</div>{/if}
</div>

<style>
	.crumbs {
		font-size: 12.5px;
		margin-bottom: 2px;
	}
	.min {
		min-width: 0;
	}
</style>
