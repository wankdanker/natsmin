<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api, unwrap, errMsg, type Schemas } from '$lib/api/client';
	import { can } from '$lib/session.svelte';
	import { toast } from '$lib/toast.svelte';
	import { findSystem, refreshSystems } from '$lib/systems.svelte';
	import { confirm } from '$lib/components/Confirm.svelte';
	import { ago } from '$lib/format';
	import SystemForm from '$lib/components/SystemForm.svelte';
	import CredentialFields from '$lib/components/CredentialFields.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import Empty from '$lib/components/Empty.svelte';

	type Account = Schemas['AccountOut'];
	const id = $derived(page.params.system!);
	const sys = $derived(findSystem(id));

	let accOpen = $state(false);
	let accEditing = $state<Account | null>(null);
	let accName = $state('');
	let accId = $state('');
	let accAuth = $state<Schemas['CredentialIn']>({ type: 'userpass' });
	let busy = $state(false);
	let test = $state<Schemas['TestResult'] | null>(null);

	function openAccount(a: Account | null) {
		accEditing = a;
		accName = a?.name ?? '';
		accId = a?.id ?? '';
		accAuth = { type: (a?.auth.type ?? 'userpass') as Schemas['CredentialIn']['type'], username: a?.auth.username };
		test = null;
		accOpen = true;
	}

	async function saveAccount(e: SubmitEvent) {
		e.preventDefault();
		busy = true;
		try {
			if (accEditing) {
				await unwrap(api.PUT('/v1/systems/{system}/accounts/{account}', { params: { path: { system: id, account: accEditing.id } }, body: { name: accName, auth: accAuth } }));
			} else {
				await unwrap(api.POST('/v1/systems/{system}/accounts', { params: { path: { system: id } }, body: { id: accId || undefined, name: accName, auth: accAuth } }));
			}
			await refreshSystems();
			toast('Account connection saved');
			accOpen = false;
		} catch (err) {
			toast(errMsg(err), 'bad');
		} finally {
			busy = false;
		}
	}

	async function testAccount() {
		if (!sys) return;
		try {
			test = await unwrap(
				api.POST('/v1/systems/test', { body: { systemId: id, accountId: accEditing?.id, urls: sys.urls, auth: accAuth } })
			);
		} catch (e) {
			test = { ok: false, error: errMsg(e), isSystemAccount: false, serversFound: 0, jetstream: false };
		}
	}

	async function deleteAccount(a: Account) {
		if (!(await confirm({ title: 'Remove account connection', message: `Remove ${a.name}? Nothing changes in NATS; natsmin just stops using these credentials.`, confirmText: 'Remove', danger: true })))
			return;
		try {
			await unwrap(api.DELETE('/v1/systems/{system}/accounts/{account}', { params: { path: { system: id, account: a.id } } }));
			await refreshSystems();
			toast(`Removed ${a.name}`);
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}

	async function deleteSystem() {
		if (!(await confirm({ title: 'Remove system', message: `Remove ${sys?.name} from natsmin? The NATS servers are not touched.`, confirmText: 'Remove system', danger: true, typeToConfirm: id })))
			return;
		try {
			await unwrap(api.DELETE('/v1/systems/{system}', { params: { path: { system: id } } }));
			await refreshSystems();
			toast('System removed');
			goto('/');
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}
</script>

<svelte:head><title>Settings · {sys?.name ?? id} · natsmin</title></svelte:head>

<div class="page" style="max-width:980px">
	<div class="page-head">
		<div>
			<h1>{sys?.name ?? id} settings</h1>
			<div class="sub">Connection details, credentials and account connections.</div>
		</div>
	</div>

	{#if !can('admin')}
		<div class="alert info">Only admins can change system settings.</div>
	{:else if sys}
		<div class="stack">
			<div class="card">
				<div class="card-head">
					<div>
						<h2>Account connections</h2>
						<div class="faint" style="font-size:12.5px">Users natsmin connects as for streams, KV, object stores and messaging. Add one per NATS account you want to manage.</div>
					</div>
					<span class="spacer"></span>
					<button class="btn" onclick={() => openAccount(null)}><Icon name="plus" />Add</button>
				</div>
				{#if sys.accounts.length === 0}
					<Empty icon="users" title="No account connections" />
				{:else}
					<table class="t">
						<thead><tr><th>Name</th><th>ID</th><th>Authentication</th><th>Updated</th><th></th></tr></thead>
						<tbody>
							{#each sys.accounts as a (a.id)}
								<tr>
									<td><b>{a.name}</b></td>
									<td class="mono">{a.id}</td>
									<td>{a.auth.type}{a.auth.username ? ` · ${a.auth.username}` : ''}{a.auth.credsUser ? ` · ${a.auth.credsUser}` : ''}</td>
									<td>{ago(a.updatedAt)}</td>
									<td class="r nowrap">
										<button class="btn sm" onclick={() => openAccount(a)}><Icon name="edit" size={13} /></button>
										<button class="btn sm danger" onclick={() => deleteAccount(a)}><Icon name="trash" size={13} /></button>
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				{/if}
			</div>

			{#key sys.updatedAt}
				<SystemForm
					existing={sys}
					onsaved={async () => {
						await refreshSystems();
						toast('Settings saved');
					}}
				/>
			{/key}

			<div class="card danger-zone">
				<div class="card-head"><h2>Remove system</h2></div>
				<div class="card-body row">
					<span class="muted">Stops monitoring and forgets the stored credentials. Your NATS servers are not touched.</span>
					<span class="spacer"></span>
					<button class="btn danger" onclick={deleteSystem}><Icon name="trash" />Remove system</button>
				</div>
			</div>
		</div>
	{/if}
</div>

<Modal bind:open={accOpen} title={accEditing ? `Edit ${accEditing.name}` : 'Add account connection'}>
	<form id="accform" class="stack" onsubmit={saveAccount}>
		<div class="grid-2">
			<label class="field"><span>Name</span><input bind:value={accName} required placeholder="Orders team" /></label>
			<label class="field"><span>ID</span><input bind:value={accId} disabled={!!accEditing} placeholder="derived from name" /></label>
		</div>
		<CredentialFields bind:value={accAuth} stored={accEditing?.auth} idPrefix="acc" />
		<div class="row"><button type="button" class="btn" onclick={testAccount}><Icon name="zap" />Test connection</button></div>
		{#if test}
			{#if test.ok}
				<div class="alert info">
					Connected to {test.serverName} (v{test.serverVersion}). JetStream {test.jetstream ? `available${test.jetstreamDomain ? ` in domain ${test.jetstreamDomain}` : ''}` : 'not enabled for this account'}.
				</div>
			{:else}<div class="alert bad">{test.error}</div>{/if}
		{/if}
	</form>
	{#snippet footer()}
		<button class="btn" onclick={() => (accOpen = false)}>Cancel</button>
		<button class="btn primary" form="accform" disabled={busy}>Save</button>
	{/snippet}
</Modal>

<style>
	.danger-zone {
		border-color: color-mix(in srgb, var(--bad) 35%, var(--border));
	}
</style>
