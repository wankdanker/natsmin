<script lang="ts">
	let { size = 24 }: { size?: number } = $props();
</script>

<svg width={size} height={size} viewBox="0 0 32 32" aria-hidden="true" style="flex:none">
	<rect width="32" height="32" rx="7" fill="var(--accent)" />
	<path d="M10 11 22 11 16 22Z" fill="none" stroke="var(--accent-text)" stroke-width="1.8" stroke-linejoin="round" opacity="0.7" />
	<circle cx="10" cy="11" r="3.2" fill="var(--accent-text)" />
	<circle cx="22" cy="11" r="3.2" fill="var(--accent-text)" />
	<circle cx="16" cy="22" r="3.2" fill="var(--accent-text)" />
</svg>
