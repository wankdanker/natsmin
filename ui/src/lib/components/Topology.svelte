<script lang="ts">
	import type { Schemas } from '$lib/api/client';
	import { compact } from '$lib/format';

	type Snapshot = Schemas['Snapshot'];
	type Server = Schemas['ServerNode'];

	let {
		snapshot,
		selected = $bindable<string | null>(null),
		height = 560
	}: { snapshot: Snapshot; selected?: string | null; height?: number } = $props();

	let width = $state(900);
	let zoom = $state(1);
	let pan = $state({ x: 0, y: 0 });
	let drag = $state<{ x: number; y: number; px: number; py: number } | null>(null);

	type Pos = { x: number; y: number };

	const layout = $derived.by(() => {
		const servers = snapshot.servers ?? [];
		const byCluster = new Map<string, Server[]>();
		for (const s of servers) {
			const c = s.cluster || s.name;
			if (!byCluster.has(c)) byCluster.set(c, []);
			byCluster.get(c)!.push(s);
		}
		const clusters = [...byCluster.keys()].sort();
		const k = clusters.length;
		const cx = width / 2;
		const cy = height / 2;
		const radius = (n: number) => (n <= 1 ? 46 : Math.max(70, 26 + n * 16));
		const maxR = Math.max(...clusters.map((c) => radius(byCluster.get(c)!.length)), 46);
		const rx = Math.max(width / 2 - maxR - 70, 0);
		const ry = Math.max(height / 2 - maxR - 50, 0);

		const centers = new Map<string, Pos & { r: number }>();
		clusters.forEach((c, i) => {
			const r = radius(byCluster.get(c)!.length);
			if (k === 1) centers.set(c, { x: cx, y: cy, r });
			else {
				// Two clusters sit side by side; more go around an ellipse.
				const a = k === 2 ? Math.PI + i * Math.PI : -Math.PI / 2 + (i / k) * Math.PI * 2;
				centers.set(c, { x: cx + rx * Math.cos(a), y: cy + ry * Math.sin(a), r });
			}
		});

		const pos = new Map<string, Pos>();
		for (const c of clusters) {
			const list = byCluster.get(c)!.sort((a, b) => a.name.localeCompare(b.name));
			const ctr = centers.get(c)!;
			list.forEach((s, i) => {
				if (list.length === 1) pos.set(s.id, { x: ctr.x, y: ctr.y });
				else {
					const a = -Math.PI / 2 + (i / list.length) * Math.PI * 2;
					const rr = ctr.r - 26;
					pos.set(s.id, { x: ctr.x + rr * Math.cos(a), y: ctr.y + rr * Math.sin(a) });
				}
			});
		}

		// Routes: expected full mesh within each cluster.
		const routeSet = new Set<string>();
		for (const r of snapshot.routes ?? []) routeSet.add([r.from, r.to].sort().join('|'));
		const routeLines: { a: Pos; b: Pos; ok: boolean; key: string }[] = [];
		for (const c of clusters) {
			const list = byCluster.get(c)!.filter((s) => !s.missing);
			for (let i = 0; i < list.length; i++)
				for (let j = i + 1; j < list.length; j++) {
					const key = [list[i].id, list[j].id].sort().join('|');
					routeLines.push({ a: pos.get(list[i].id)!, b: pos.get(list[j].id)!, ok: routeSet.has(key), key });
				}
		}

		// Gateways: one link per cluster pair.
		const gw = new Map<string, { a: string; b: string; connected: number; total: number; msgs: number }>();
		for (const g of snapshot.gateways ?? []) {
			if (!centers.has(g.fromCluster) || !centers.has(g.toCluster) || g.fromCluster === g.toCluster) continue;
			const [a, b] = [g.fromCluster, g.toCluster].sort();
			const key = a + '|' + b;
			const e = gw.get(key) ?? { a, b, connected: 0, total: 0, msgs: 0 };
			e.total++;
			if (g.connected) e.connected++;
			e.msgs += (g.inMsgs ?? 0) + (g.outMsgs ?? 0);
			gw.set(key, e);
		}
		const gwLines = [...gw.values()].map((e) => {
			const A = centers.get(e.a)!;
			const B = centers.get(e.b)!;
			const dx = B.x - A.x;
			const dy = B.y - A.y;
			const d = Math.hypot(dx, dy) || 1;
			return {
				...e,
				x1: A.x + (dx / d) * A.r,
				y1: A.y + (dy / d) * A.r,
				x2: B.x - (dx / d) * B.r,
				y2: B.y - (dy / d) * B.r,
				mx: (A.x + B.x) / 2,
				my: (A.y + B.y) / 2
			};
		});

		// Leafnodes: placed outward from their hub server.
		const perServer = new Map<string, number>();
		const leaves = (snapshot.leafnodes ?? []).map((l) => {
			const hub = pos.get(l.server);
			const srv = servers.find((s) => s.id === l.server);
			const ctr = centers.get(srv?.cluster || srv?.name || '') ?? { x: cx, y: cy };
			const n = perServer.get(l.server) ?? 0;
			perServer.set(l.server, n + 1);
			if (!hub) return null;
			let ang = Math.atan2(hub.y - ctr.y, hub.x - ctr.x);
			if (hub.x === ctr.x && hub.y === ctr.y) ang = Math.PI / 2;
			ang += (n - 0.5) * 0.45 * (n % 2 ? 1 : -1);
			const dist = 70 + n * 8;
			return { leaf: l, hub, x: hub.x + Math.cos(ang) * dist, y: hub.y + Math.sin(ang) * dist };
		});

		return { clusters, centers, pos, routeLines, gwLines, leaves: leaves.filter((x) => x !== null), byCluster };
	});

	const metaLeader = $derived(snapshot.jetstream?.metaLeader);

	function tone(s: Server) {
		if (s.missing) return 'missing';
		if (!s.healthy) return 'bad';
		if ((s.slowConsumers ?? 0) > 0 || s.cpu > 85) return 'warn';
		return 'ok';
	}

	function onwheel(e: WheelEvent) {
		e.preventDefault();
		const f = e.deltaY < 0 ? 1.1 : 1 / 1.1;
		zoom = Math.min(3, Math.max(0.4, zoom * f));
	}
	function ondown(e: PointerEvent) {
		if ((e.target as Element).closest('.node')) return;
		drag = { x: e.clientX, y: e.clientY, px: pan.x, py: pan.y };
		(e.currentTarget as Element).setPointerCapture(e.pointerId);
	}
	function onmove(e: PointerEvent) {
		if (!drag) return;
		pan = { x: drag.px + (e.clientX - drag.x) / zoom, y: drag.py + (e.clientY - drag.y) / zoom };
	}
	function onup() {
		drag = null;
	}
	function reset() {
		zoom = 1;
		pan = { x: 0, y: 0 };
	}
</script>

<div class="topo" bind:clientWidth={width}>
	<div class="controls">
		<button class="btn sm" onclick={() => (zoom = Math.min(3, zoom * 1.2))} aria-label="Zoom in">+</button>
		<button class="btn sm" onclick={() => (zoom = Math.max(0.4, zoom / 1.2))} aria-label="Zoom out">−</button>
		<button class="btn sm" onclick={reset}>Reset</button>
	</div>
	<svg
		{width}
		{height}
		role="img"
		aria-label="System topology"
		{onwheel}
		onpointerdown={ondown}
		onpointermove={onmove}
		onpointerup={onup}
		onpointercancel={onup}
		class:dragging={!!drag}
	>
		<g transform="translate({width / 2} {height / 2}) scale({zoom}) translate({-width / 2 + pan.x} {-height / 2 + pan.y})">
			{#each layout.clusters as c}
				{@const ctr = layout.centers.get(c)!}
				{#if layout.byCluster.get(c)!.length > 1 || (snapshot.clusters?.length ?? 0) > 1}
					<circle cx={ctr.x} cy={ctr.y} r={ctr.r} class="cluster" />
					<text x={ctr.x} y={ctr.y - ctr.r - 8} text-anchor="middle" class="cluster-label">{c}</text>
				{/if}
			{/each}

			{#each layout.gwLines as g}
				<line x1={g.x1} y1={g.y1} x2={g.x2} y2={g.y2} class="gw" class:degraded={g.connected < g.total} class:flow={g.msgs > 0} />
				<g transform="translate({g.mx} {g.my})">
					<rect x="-46" y="-11" width="92" height="22" rx="11" class="gw-pill" class:degraded={g.connected < g.total} />
					<text text-anchor="middle" y="4" class="gw-text">gateway {g.connected}/{g.total}</text>
				</g>
			{/each}

			{#each layout.routeLines as r (r.key)}
				<line x1={r.a.x} y1={r.a.y} x2={r.b.x} y2={r.b.y} class="route" class:broken={!r.ok} />
			{/each}

			{#each layout.leaves as l}
				<line x1={l.hub.x} y1={l.hub.y} x2={l.x} y2={l.y} class="leaf-link" />
				<g class="leaf" transform="translate({l.x} {l.y})">
					<rect x="-7" y="-7" width="14" height="14" rx="3" transform="rotate(45)" />
					<text y="22" text-anchor="middle" class="leaf-label">{l.leaf.name || l.leaf.ip}</text>
					<title>Leafnode {l.leaf.name} ({l.leaf.ip}:{l.leaf.port}) account {l.leaf.account} · rtt {l.leaf.rtt}</title>
				</g>
			{/each}

			{#each snapshot.servers ?? [] as s (s.id)}
				{@const p = layout.pos.get(s.id)}
				{#if p}
					<g
						class="node {tone(s)}"
						class:sel={selected === s.id}
						transform="translate({p.x} {p.y})"
						role="button"
						tabindex="0"
						onclick={() => (selected = selected === s.id ? null : s.id)}
						onkeydown={(e) => e.key === 'Enter' && (selected = s.id)}
					>
						<circle r="17" class="halo" />
						<circle r="13" class="body" />
						{#if s.jetstream}
							<text y="4" text-anchor="middle" class="js">{s.name === metaLeader ? '★' : 'JS'}</text>
						{/if}
						<text y="31" text-anchor="middle" class="label">{s.name}</text>
						<text y="44" text-anchor="middle" class="label faint-l">{compact(s.connections)} conns</text>
						<title>{s.name} · {s.version} · {s.healthy ? 'healthy' : s.healthError || 'unhealthy'}</title>
					</g>
				{/if}
			{/each}
		</g>
	</svg>
	<div class="legend">
		<span><i class="lg ok"></i>healthy</span>
		<span><i class="lg warn"></i>warning</span>
		<span><i class="lg bad"></i>unhealthy</span>
		<span><i class="lg missing"></i>not answering</span>
		<span><i class="ln route"></i>route</span>
		<span><i class="ln broken"></i>missing route</span>
		<span><i class="ln gw"></i>gateway</span>
		<span><i class="ln leaf"></i>leafnode</span>
		<span>★ JetStream meta leader</span>
	</div>
</div>

<style>
	.topo {
		position: relative;
		width: 100%;
		user-select: none;
	}
	svg {
		display: block;
		cursor: grab;
		background:
			radial-gradient(circle at 1px 1px, var(--border) 1px, transparent 0) 0 0 / 22px 22px,
			var(--bg-sunken);
		border-radius: var(--radius-sm);
	}
	svg.dragging {
		cursor: grabbing;
	}
	.controls {
		position: absolute;
		top: 10px;
		right: 10px;
		display: flex;
		gap: 4px;
		z-index: 2;
	}
	.cluster {
		fill: color-mix(in srgb, var(--accent) 5%, var(--bg-elev));
		stroke: var(--border-strong);
		stroke-dasharray: 4 4;
	}
	.cluster-label {
		fill: var(--text-2);
		font-size: 12px;
		font-weight: 600;
		letter-spacing: 0.04em;
		text-transform: uppercase;
	}
	.route {
		stroke: var(--border-strong);
		stroke-width: 1.3;
	}
	.route.broken {
		stroke: var(--bad);
		stroke-dasharray: 4 3;
		stroke-width: 1.6;
	}
	.gw {
		stroke: var(--series-2);
		stroke-width: 3;
		opacity: 0.8;
	}
	.gw.flow {
		stroke-dasharray: 10 6;
		animation: flow 1.2s linear infinite;
	}
	.gw.degraded {
		stroke: var(--warn);
	}
	@keyframes flow {
		to {
			stroke-dashoffset: -32;
		}
	}
	.gw-pill {
		fill: var(--bg-elev);
		stroke: var(--series-2);
	}
	.gw-pill.degraded {
		stroke: var(--warn);
	}
	.gw-text {
		font-size: 10.5px;
		fill: var(--text-2);
	}
	.leaf-link {
		stroke: var(--series-3);
		stroke-dasharray: 3 3;
		stroke-width: 1.4;
	}
	.leaf rect {
		fill: var(--bg-elev);
		stroke: var(--series-3);
		stroke-width: 1.6;
	}
	.leaf-label {
		font-size: 10.5px;
		fill: var(--text-2);
	}
	.node {
		cursor: pointer;
		outline: none;
	}
	.node .halo {
		fill: transparent;
		stroke: transparent;
		stroke-width: 3;
	}
	.node.sel .halo,
	.node:focus-visible .halo {
		stroke: var(--accent);
	}
	.node .body {
		stroke-width: 2;
		fill: var(--bg-elev);
	}
	.node.ok .body {
		stroke: var(--ok);
		fill: color-mix(in srgb, var(--ok) 16%, var(--bg-elev));
	}
	.node.warn .body {
		stroke: var(--warn);
		fill: color-mix(in srgb, var(--warn) 16%, var(--bg-elev));
	}
	.node.bad .body {
		stroke: var(--bad);
		fill: color-mix(in srgb, var(--bad) 18%, var(--bg-elev));
	}
	.node.missing .body {
		stroke: var(--text-3);
		stroke-dasharray: 3 2;
	}
	.node:hover .body {
		stroke-width: 3;
	}
	.js {
		font-size: 9px;
		font-weight: 700;
		fill: var(--text);
	}
	.label {
		font-size: 11.5px;
		fill: var(--text);
		font-weight: 500;
	}
	.faint-l {
		fill: var(--text-3);
		font-weight: 400;
		font-size: 10.5px;
	}
	.legend {
		display: flex;
		flex-wrap: wrap;
		gap: 14px;
		font-size: 12px;
		color: var(--text-2);
		padding-top: 10px;
	}
	.legend span {
		display: inline-flex;
		align-items: center;
		gap: 6px;
	}
	.lg {
		width: 10px;
		height: 10px;
		border-radius: 50%;
		border: 2px solid;
	}
	.lg.ok {
		border-color: var(--ok);
	}
	.lg.warn {
		border-color: var(--warn);
	}
	.lg.bad {
		border-color: var(--bad);
	}
	.lg.missing {
		border-color: var(--text-3);
		border-style: dashed;
	}
	.ln {
		width: 18px;
		height: 0;
		border-top: 2px solid;
	}
	.ln.route {
		border-color: var(--border-strong);
	}
	.ln.broken {
		border-color: var(--bad);
		border-top-style: dashed;
	}
	.ln.gw {
		border-color: var(--series-2);
		border-top-width: 3px;
	}
	.ln.leaf {
		border-color: var(--series-3);
		border-top-style: dashed;
	}
</style>
