<script>
	import { api } from '$lib/api';

	export let title = '';
	export let year = 0;
	export let posterUrl = '';
	export let rating = 0;
	export let status = '';
	export let inLibrary = false;
	export let anime = false;

	$: proxiedPoster = posterUrl ? api.imageUrl(posterUrl) : '';
</script>

<div class="card" on:click on:keydown role="button" tabindex="0">
	<div class="poster">
		{#if proxiedPoster}
			<img src={proxiedPoster} alt={title} loading="lazy" />
		{:else}
			<div class="no-poster">
				<span>{title.charAt(0)}</span>
			</div>
		{/if}
		{#if rating > 0}
			<div class="rating">{rating.toFixed(1)}</div>
		{/if}
		{#if inLibrary && status}
			<div class="status-badge status-{status}">
				{status}
			</div>
		{/if}
		{#if anime}
			<div class="anime-badge">ANIME</div>
		{/if}
	</div>
	<div class="info">
		<div class="title" title={title}>{title}</div>
		{#if year}
			<div class="year">{year}</div>
		{/if}
	</div>
</div>

<style>
	.card {
		cursor: pointer;
		transition: transform 0.15s, box-shadow 0.15s;
		border-radius: 10px;
		overflow: hidden;
		background: var(--bg-surface);
		backdrop-filter: blur(12px);
		-webkit-backdrop-filter: blur(12px);
	}

	.card:hover {
		transform: translateY(-4px);
		box-shadow: var(--shadow-md);
	}

	.poster {
		position: relative;
		aspect-ratio: 2/3;
		background: var(--bg-elevated);
		overflow: hidden;
	}

	.poster img {
		width: 100%;
		height: 100%;
		object-fit: cover;
	}

	.no-poster {
		width: 100%;
		height: 100%;
		display: flex;
		align-items: center;
		justify-content: center;
		background: var(--bg-elevated);
		font-size: 3rem;
		color: var(--text-dim);
	}

	.rating {
		position: absolute;
		top: 6px;
		right: 6px;
		background: var(--bg-overlay);
		color: var(--gold);
		padding: 2px 6px;
		border-radius: 4px;
		font-size: 0.75rem;
		font-weight: 600;
	}

	.status-badge {
		position: absolute;
		bottom: 6px;
		left: 6px;
		padding: 2px 8px;
		border-radius: 4px;
		font-size: 0.65rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.5px;
	}

	.status-badge.status-wanted {
		background: var(--status-wanted);
	}

	.status-badge.status-searching,
	.status-badge.status-downloading {
		background: var(--status-searching);
	}

	.status-badge.status-available {
		background: var(--status-available);
	}

	.status-badge.status-unavailable {
		background: var(--status-unavailable);
	}

	.anime-badge {
		position: absolute;
		top: 6px;
		left: 6px;
		background: rgba(0, 212, 255, 0.85);
		color: white;
		padding: 2px 6px;
		border-radius: 4px;
		font-size: 0.6rem;
		font-weight: 700;
		letter-spacing: 0.5px;
	}

	.info {
		padding: 0.5rem 0.6rem;
	}

	.title {
		font-size: 0.825rem;
		font-weight: 500;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		color: var(--text-primary);
	}

	.year {
		font-size: 0.75rem;
		color: var(--text-muted);
		margin-top: 2px;
	}
</style>
