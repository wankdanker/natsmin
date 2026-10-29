import { api, unwrap, type Schemas } from './api/client';

type Snapshot = Schemas['Snapshot'];
type RatePoint = Schemas['RatePoint'];

/** Live view of one system: the latest snapshot plus rate history, kept current over SSE. */
export class LiveSystem {
	id: string;
	snapshot = $state<Snapshot | null>(null);
	history = $state<RatePoint[]>([]);
	connected = $state(false);
	error = $state<string | null>(null);
	#es: EventSource | null = null;
	#retry: ReturnType<typeof setTimeout> | null = null;
	#closed = false;

	constructor(id: string) {
		this.id = id;
	}

	async start() {
		this.#closed = false;
		try {
			this.history = await unwrap(api.GET('/v1/systems/{system}/history', { params: { path: { system: this.id } } }));
		} catch {}
		this.#open();
	}

	#open() {
		if (this.#closed) return;
		const es = new EventSource(`/api/v1/systems/${encodeURIComponent(this.id)}/events`);
		this.#es = es;
		es.addEventListener('snapshot', (e) => {
			const snap = JSON.parse((e as MessageEvent).data) as Snapshot;
			this.snapshot = snap;
			this.connected = true;
			this.error = snap.error || null;
			if (snap.connected) {
				const last = this.history[this.history.length - 1];
				if (!last || last.t !== snap.time) {
					this.history.push({
						t: snap.time,
						inMsgs: snap.totals.inMsgsRate,
						outMsgs: snap.totals.outMsgsRate,
						inBytes: snap.totals.inBytesRate,
						outBytes: snap.totals.outBytesRate,
						connections: snap.totals.connections,
						subscriptions: snap.totals.subscriptions,
						servers: snap.totals.servers
					});
					if (this.history.length > 720) this.history.splice(0, this.history.length - 720);
				}
			}
		});
		es.onerror = () => {
			this.connected = false;
			es.close();
			if (!this.#closed) this.#retry = setTimeout(() => this.#open(), 3000);
		};
	}

	async refresh() {
		this.snapshot = await unwrap(
			api.GET('/v1/systems/{system}/snapshot', { params: { path: { system: this.id }, query: { refresh: true } } })
		);
	}

	close() {
		this.#closed = true;
		this.#es?.close();
		if (this.#retry) clearTimeout(this.#retry);
	}
}
