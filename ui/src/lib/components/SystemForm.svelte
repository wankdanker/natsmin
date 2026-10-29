<script lang="ts">
	import { untrack } from 'svelte';
	import { api, unwrap, errMsg, type Schemas } from '$lib/api/client';
	import CredentialFields from './CredentialFields.svelte';
	import Icon from './Icon.svelte';

	type SystemOut = Schemas['SystemOut'];
	let {
		existing = null,
		onsaved
	}: { existing?: SystemOut | null; onsaved: (s: SystemOut) => void } = $props();

	// The form edits a copy of the system as it was when opened.
	const ex = untrack(() => existing);
	const isNew = !ex;
	let name = $state(ex?.name ?? '');
	let id = $state(ex?.id ?? '');
	let description = $state(ex?.description ?? '');
	let urls = $state((ex?.urls ?? ['nats://localhost:4222']).join('\n'));
	let useTLS = $state(!!ex?.tls);
	let tls = $state<Schemas['TLSIn']>({
		ca: ex?.tls?.ca ?? '',
		cert: ex?.tls?.cert ?? '',
		key: '',
		insecureSkipVerify: ex?.tls?.insecureSkipVerify ?? false
	});
	let useSys = $state(isNew || !!ex?.systemAuth);
	let sysAuth = $state<Schemas['CredentialIn']>({
		type: (ex?.systemAuth?.type ?? 'userpass') as Schemas['CredentialIn']['type'],
		username: ex?.systemAuth?.username ?? ''
	});
	let addAccount = $state(isNew);
	let accName = $state('Default');
	let accAuth = $state<Schemas['CredentialIn']>({ type: 'userpass', username: '' });

	let busy = $state(false);
	let error = $state<string | null>(null);
	let testing = $state<'sys' | 'acc' | null>(null);
	let testResult = $state<{ which: 'sys' | 'acc'; r: Schemas['TestResult'] } | null>(null);

	const urlList = $derived(urls.split(/[\n,]/).map((u) => u.trim()).filter(Boolean));
	const autoId = $derived(
		name
			.toLowerCase()
			.replace(/[^a-z0-9]+/g, '-')
			.replace(/^-+|-+$/g, '')
			.slice(0, 63)
	);

	async function test(which: 'sys' | 'acc') {
		testing = which;
		testResult = null;
		try {
			const r = await unwrap(
				api.POST('/v1/systems/test', {
					body: {
						systemId: existing?.id,
						urls: urlList,
						tls: useTLS ? tls : undefined,
						auth: which === 'sys' ? sysAuth : accAuth
					}
				})
			);
			testResult = { which, r };
		} catch (e) {
			testResult = { which, r: { ok: false, error: errMsg(e), isSystemAccount: false, serversFound: 0, jetstream: false } };
		} finally {
			testing = null;
		}
	}

	async function save(e: SubmitEvent) {
		e.preventDefault();
		busy = true;
		error = null;
		try {
			const body: Schemas['SystemIn'] = {
				name,
				description,
				urls: urlList,
				tls: useTLS ? tls : undefined,
				removeTls: !useTLS && !!existing?.tls,
				systemAuth: useSys ? sysAuth : undefined,
				removeSystemAuth: !useSys && !!existing?.systemAuth
			};
			let saved: SystemOut;
			if (isNew) {
				body.id = id || undefined;
				if (addAccount) body.accounts = [{ name: accName, auth: accAuth }];
				saved = await unwrap(api.POST('/v1/systems', { body }));
			} else {
				saved = await unwrap(api.PUT('/v1/systems/{system}', { params: { path: { system: existing!.id } }, body }));
			}
			onsaved(saved);
		} catch (err) {
			error = errMsg(err);
		} finally {
			busy = false;
		}
	}
</script>

{#snippet result(which: 'sys' | 'acc')}
	{#if testResult?.which === which}
		{@const r = testResult.r}
		{#if !r.ok}
			<div class="alert bad">Connection failed: {r.error}</div>
		{:else}
			<div class="alert {which === 'sys' && !r.isSystemAccount ? 'warn' : 'info'}">
				Connected to <b>{r.serverName}</b> (v{r.serverVersion}{r.cluster ? `, cluster ${r.cluster}` : ''}) in {r.rtt}.
				{#if which === 'sys'}
					{#if r.isSystemAccount}System account confirmed: {r.serversFound} server{r.serversFound === 1 ? '' : 's'} answered.
					{:else}These credentials are <b>not</b> in the system account, so monitoring will not work.{/if}
				{:else}
					JetStream {r.jetstream ? `available${r.jetstreamDomain ? ` (domain ${r.jetstreamDomain})` : ''}` : 'not enabled for this account'}.
				{/if}
			</div>
		{/if}
	{/if}
{/snippet}

<form onsubmit={save} class="stack">
	{#if error}<div class="alert bad">{error}</div>{/if}

	<div class="card">
		<div class="card-head"><h2>Connection</h2></div>
		<div class="card-body stack">
			<div class="grid-2">
				<label class="field"><span>Name</span><input bind:value={name} required placeholder="Production" /></label>
				{#if isNew}
					<label class="field"
						><span>ID</span><input bind:value={id} placeholder={autoId || 'production'} pattern="[a-z0-9][a-z0-9-]*" />
						<span class="hint">Used in URLs and the API. Cannot change later.</span></label
					>
				{:else}
					<label class="field"><span>ID</span><input value={existing?.id} disabled /></label>
				{/if}
			</div>
			<label class="field"><span>Description</span><input bind:value={description} placeholder="Optional" /></label>
			<label class="field">
				<span>Server URLs</span>
				<textarea rows="3" bind:value={urls} required placeholder="nats://host-1:4222&#10;nats://host-2:4222"></textarea>
				<span class="hint">One per line. Seed URLs; the rest of the cluster is discovered.</span>
			</label>
			<label class="check"><input type="checkbox" bind:checked={useTLS} />Custom TLS settings</label>
			{#if useTLS}
				<div class="grid-3">
					<label class="field"><span>CA certificate (PEM)</span><textarea rows="4" bind:value={tls.ca}></textarea></label>
					<label class="field"><span>Client certificate (PEM)</span><textarea rows="4" bind:value={tls.cert}></textarea></label>
					<label class="field"
						><span>Client key (PEM)</span><textarea
							rows="4"
							bind:value={tls.key}
							placeholder={existing?.tls?.hasKey ? 'Stored · leave blank to keep' : ''}
						></textarea></label
					>
				</div>
				<label class="check"><input type="checkbox" bind:checked={tls.insecureSkipVerify} />Skip certificate verification (not recommended)</label>
			{/if}
		</div>
	</div>

	<div class="card">
		<div class="card-head">
			<div>
				<h2>System account</h2>
				<div class="faint" style="font-size:12.5px">Topology, health, connections and maintenance use the system account.</div>
			</div>
			<span class="spacer"></span>
			<label class="check"><input type="checkbox" bind:checked={useSys} />Enabled</label>
		</div>
		{#if useSys}
			<div class="card-body stack">
				<CredentialFields bind:value={sysAuth} stored={existing?.systemAuth} idPrefix="sys" />
				<div class="row">
					<button type="button" class="btn" onclick={() => test('sys')} disabled={testing !== null || urlList.length === 0}>
						{#if testing === 'sys'}<span class="spin"></span>{:else}<Icon name="zap" />{/if}Test connection
					</button>
				</div>
				{@render result('sys')}
			</div>
		{/if}
	</div>

	{#if isNew}
		<div class="card">
			<div class="card-head">
				<div>
					<h2>Account connection</h2>
					<div class="faint" style="font-size:12.5px">Streams, KV, object stores and messaging run as a user in a regular account. You can add more later.</div>
				</div>
				<span class="spacer"></span>
				<label class="check"><input type="checkbox" bind:checked={addAccount} />Add one now</label>
			</div>
			{#if addAccount}
				<div class="card-body stack">
					<label class="field"><span>Name</span><input bind:value={accName} required /></label>
					<CredentialFields bind:value={accAuth} idPrefix="acc" />
					<div class="row">
						<button type="button" class="btn" onclick={() => test('acc')} disabled={testing !== null || urlList.length === 0}>
							{#if testing === 'acc'}<span class="spin"></span>{:else}<Icon name="zap" />{/if}Test connection
						</button>
					</div>
					{@render result('acc')}
				</div>
			{/if}
		</div>
	{/if}

	<div class="row">
		<button class="btn primary" disabled={busy}>{busy ? 'Saving…' : isNew ? 'Add system' : 'Save changes'}</button>
	</div>
</form>
