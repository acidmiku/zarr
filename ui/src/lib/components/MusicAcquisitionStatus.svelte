<script>
	import { acquisitionState, acquisitionLabels } from '$lib/music';
	export let album;
	export let compact = false;
	$: state = acquisitionState(album);
	$: description =
		state.message ||
		{
			saved: 'Saved to your library. Download whenever you choose.',
			no_results: 'Sources were searched, but no release matched this album and profile.',
			blocked: 'A music source or download client needs attention in Settings.',
			failed: 'The transfer or import did not finish. Retry to check the next step.',
			cancelled: 'The acquisition was cancelled. Your saved album remains in the library.'
		}[state.status] ||
		'';
</script>

<div
	class="acquisition"
	class:compact
	class:attention={['failed', 'blocked', 'no_results'].includes(state.status)}
	data-acquisition={state.status}
>
	<span class="acquisition-label">{acquisitionLabels[state.status] || state.status}</span>
	{#if !compact && description}<p>{description}</p>{/if}
	{#if !compact && state.status === 'blocked'}<a href="/settings"
			>Review music sources & download clients</a
		>{/if}
</div>

<style>
	.acquisition {
		font-size: 0.8rem;
		line-height: 1.6;
		color: var(--text-secondary);
	}
	.acquisition-label {
		display: inline-flex;
		align-items: center;
		gap: 0.45rem;
		font-size: 0.7rem;
		font-weight: 650;
		color: var(--text-primary);
	}
	.acquisition-label::before {
		content: '';
		width: 6px;
		height: 6px;
		border-radius: 50%;
		background: var(--accent);
		flex-shrink: 0;
	}
	.attention .acquisition-label::before {
		background: var(--warning, #edae49);
	}
	p {
		margin: 0.4rem 0 0;
		overflow-wrap: anywhere;
	}
	a {
		display: inline-block;
		color: var(--accent);
		margin-top: 0.35rem;
	}
	.compact {
		line-height: 1.3;
	}
</style>
