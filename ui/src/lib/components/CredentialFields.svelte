<script lang="ts">
	import type { Schemas } from '$lib/api/client';

	type CredIn = Schemas['CredentialIn'];
	let {
		value = $bindable(),
		stored = null,
		idPrefix = 'cred'
	}: { value: CredIn; stored?: Schemas['CredentialOut'] | null; idPrefix?: string } = $props();

	const keep = (has: boolean | undefined) => (stored && stored.type === value.type && has ? 'Stored · leave blank to keep' : '');
</script>

<div class="stack">
	<label class="field">
		<span>Authentication</span>
		<select bind:value={value.type} id="{idPrefix}-type">
			<option value="none">None</option>
			<option value="userpass">Username and password</option>
			<option value="token">Token</option>
			<option value="creds">Credentials file (.creds, JWT)</option>
			<option value="nkey">NKey seed</option>
		</select>
	</label>
	{#if value.type === 'userpass'}
		<div class="grid-2">
			<label class="field"><span>Username</span><input bind:value={value.username} autocomplete="off" /></label>
			<label class="field"
				><span>Password</span><input
					type="password"
					bind:value={value.password}
					autocomplete="new-password"
					placeholder={keep(stored?.hasPassword)}
				/></label
			>
		</div>
	{:else if value.type === 'token'}
		<label class="field"
			><span>Token</span><input type="password" bind:value={value.token} autocomplete="off" placeholder={keep(stored?.hasToken)} /></label
		>
	{:else if value.type === 'creds'}
		<label class="field">
			<span>Credentials file contents</span>
			<textarea
				rows="6"
				bind:value={value.creds}
				placeholder={keep(stored?.hasCreds) || '-----BEGIN NATS USER JWT-----\n...\n------END USER NKEY SEED------'}
			></textarea>
			{#if stored?.credsUser}<span class="hint">Stored user: {stored.credsUser} · account {stored.credsAccount}</span>{/if}
		</label>
	{:else if value.type === 'nkey'}
		<label class="field"
			><span>NKey seed</span><input type="password" bind:value={value.nkeySeed} autocomplete="off" placeholder={keep(stored?.hasNKey) || 'SU…'} /></label
		>
	{/if}
</div>
