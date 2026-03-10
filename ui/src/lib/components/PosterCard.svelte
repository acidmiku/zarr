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

	let cardEl;
	let tiltX = 0;
	let tiltY = 0;
	let glareX = 50;
	let glareY = 50;
	let hovering = false;

	function handleMouseMove(e) {
		if (!cardEl) return;
		const rect = cardEl.getBoundingClientRect();
		const x = (e.clientX - rect.left) / rect.width;
		const y = (e.clientY - rect.top) / rect.height;
		tiltX = (y - 0.5) * -8;
		tiltY = (x - 0.5) * 8;
		glareX = x * 100;
		glareY = y * 100;
	}

	function handleMouseEnter() { hovering = true; }
	function handleMouseLeave() { hovering = false; tiltX = 0; tiltY = 0; }
</script>

<div
	class="card"
	class:hovering
	bind:this={cardEl}
	on:click
	on:keydown
	on:mousemove={handleMouseMove}
	on:mouseenter={handleMouseEnter}
	on:mouseleave={handleMouseLeave}
	role="button"
	tabindex="0"
	style="--tilt-x: {tiltX}deg; --tilt-y: {tiltY}deg; --glare-x: {glareX}%; --glare-y: {glareY}%"
>
	<div class="poster">
		{#if proxiedPoster}
			<img src={proxiedPoster} alt={title} loading="lazy" />
		{:else}
			<div class="no-poster">
				<span>{title.charAt(0)}</span>
			</div>
		{/if}

		<div class="poster-gradient"></div>
		<div class="poster-glare"></div>

		{#if rating > 0}
			<div class="rating-badge">
				<span class="star">★</span>{rating.toFixed(1)}
			</div>
		{/if}
		{#if inLibrary && status}
			<div class="status-dot status-{status}" title={status}></div>
		{/if}
		{#if anime}
			<div class="anime-badge">ANIME</div>
		{/if}

		<div class="poster-info">
			<div class="poster-title">{title}</div>
			{#if year}<div class="poster-year">{year}</div>{/if}
		</div>
	</div>
	<div class="info">
		<div class="title" title={title}>{title}</div>
		{#if year}<div class="year">{year}</div>{/if}
	</div>
</div>

<style>
	.card {
		cursor: pointer;
		border-radius: var(--radius-md);
		overflow: hidden;
		background: var(--bg-surface);
		transform-style: preserve-3d;
		transform: perspective(800px) rotateX(var(--tilt-x)) rotateY(var(--tilt-y));
		transition: transform 0.4s cubic-bezier(0.16, 1, 0.3, 1), box-shadow 0.4s ease;
		will-change: transform;
	}
	.card.hovering {
		transition: transform 0.1s ease-out, box-shadow 0.3s ease;
		box-shadow: var(--shadow-lg);
	}
	.card:not(.hovering) { --tilt-x: 0deg; --tilt-y: 0deg; }

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
		transition: transform 0.5s cubic-bezier(0.16, 1, 0.3, 1);
	}
	.card.hovering .poster img { transform: scale(1.06); }

	.poster-gradient {
		position: absolute;
		inset: 0;
		background: linear-gradient(to top, rgba(0,0,0,0.7) 0%, rgba(0,0,0,0.15) 40%, transparent 70%);
		opacity: 0;
		transition: opacity 0.35s ease;
		pointer-events: none;
	}
	.card.hovering .poster-gradient { opacity: 1; }

	.poster-glare {
		position: absolute;
		inset: 0;
		background: radial-gradient(circle at var(--glare-x) var(--glare-y), rgba(255,255,255,0.08) 0%, transparent 60%);
		opacity: 0;
		transition: opacity 0.3s ease;
		pointer-events: none;
	}
	.card.hovering .poster-glare { opacity: 1; }

	.no-poster {
		width: 100%;
		height: 100%;
		display: flex;
		align-items: center;
		justify-content: center;
		background: var(--bg-elevated);
		font-family: var(--font-display);
		font-size: 2.5rem;
		font-weight: 700;
		color: var(--text-dim);
	}

	.rating-badge {
		position: absolute;
		top: 8px;
		right: 8px;
		background: rgba(0,0,0,0.6);
		backdrop-filter: blur(8px);
		-webkit-backdrop-filter: blur(8px);
		color: var(--gold);
		padding: 3px 8px;
		border-radius: 6px;
		font-size: 0.72rem;
		font-weight: 700;
		display: flex;
		align-items: center;
		gap: 3px;
	}
	.star { font-size: 0.6rem; }

	.status-dot {
		position: absolute;
		bottom: 10px;
		left: 10px;
		width: 8px;
		height: 8px;
		border-radius: 50%;
		box-shadow: 0 0 6px currentColor;
		z-index: 2;
	}
	.status-dot.status-wanted { background: var(--status-wanted); color: var(--status-wanted); }
	.status-dot.status-searching,
	.status-dot.status-downloading { background: var(--status-searching); color: var(--status-searching); }
	.status-dot.status-available { background: var(--status-available); color: var(--status-available); }
	.status-dot.status-unavailable { background: var(--status-unavailable); color: var(--status-unavailable); }

	.anime-badge {
		position: absolute;
		top: 8px;
		left: 8px;
		background: var(--badge-anime);
		color: white;
		padding: 2px 7px;
		border-radius: 5px;
		font-size: 0.55rem;
		font-weight: 800;
		letter-spacing: 0.06em;
	}

	.poster-info {
		position: absolute;
		bottom: 0;
		left: 0;
		right: 0;
		padding: 1.5rem 0.75rem 0.65rem;
		background: linear-gradient(transparent, rgba(0,0,0,0.85));
		opacity: 0;
		transform: translateY(4px);
		transition: opacity 0.3s ease, transform 0.3s ease;
		pointer-events: none;
		z-index: 2;
	}
	.card.hovering .poster-info { opacity: 1; transform: translateY(0); }
	.poster-title {
		font-family: var(--font-display);
		font-size: 0.8rem;
		font-weight: 700;
		color: white;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.poster-year { font-size: 0.68rem; color: rgba(255,255,255,0.6); margin-top: 2px; }

	.info { padding: 0.5rem 0.65rem 0.55rem; }
	.title {
		font-family: var(--font-display);
		font-size: 0.8rem;
		font-weight: 600;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		color: var(--text-primary);
		letter-spacing: -0.01em;
	}
	.year { font-size: 0.7rem; color: var(--text-muted); margin-top: 2px; }
</style>
