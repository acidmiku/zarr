<script>
	export let release;
	$: formats = release.matched_formats || [];
	const signed = (score) => (score > 0 ? `+${score}` : String(score));
</script>

{#if release.scoring_preset}
	<details class="score-details">
		<summary aria-label={`Explain scoring for ${release.title}`}
			><span class="score-value">{release.format_score ?? 0}</span>
			<span>format score</span></summary
		>
		<div class="score-explanation">
			<p>
				<strong>{release.quality || 'Unknown quality'}</strong> · quality rank {release.quality_rank ??
					0}
			</p>
			<p class="score-hint">
				Quality groups rank first; format scores break ties within the same group.
			</p>
			{#if formats.length}
				<ul>
					{#each formats as format}<li>
							<span>{format.name}</span><strong class:negative={format.score < 0}
								>{signed(format.score)}</strong
							>
						</li>{/each}
				</ul>
			{:else}<p>No custom formats matched.</p>{/if}
			<p class="score-version">
				{release.scoring_preset === 'radarr-anime'
					? 'TRaSH anime movies'
					: release.scoring_preset === 'sonarr-anime'
						? 'TRaSH anime series'
						: release.scoring_preset}{release.scoring_version
					? ` · ${release.scoring_version}`
					: ''}
			</p>
		</div>
	</details>
{:else}
	<div class="legacy-score" aria-label={`Release score ${release.score || 0}`}>
		{release.score || 0}
	</div>
{/if}

<style>
	.score-details {
		margin-top: 0.6rem;
		font-size: 0.78rem;
	}
	summary {
		cursor: pointer;
		color: var(--text-secondary);
		width: fit-content;
	}
	.score-value,
	.legacy-score {
		font-family: var(--font-mono);
		font-weight: 700;
		color: var(--accent);
	}
	.score-explanation {
		margin-top: 0.5rem;
		padding: 0.7rem 0.85rem;
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-sm);
		background: var(--bg-input);
	}
	p {
		margin: 0.25rem 0;
		line-height: 1.5;
	}
	.score-hint,
	.score-version {
		color: var(--text-muted);
	}
	.score-version {
		font-size: 0.7rem;
		overflow-wrap: anywhere;
	}
	ul {
		list-style: none;
		padding: 0;
		margin: 0.5rem 0;
	}
	li {
		display: flex;
		justify-content: space-between;
		gap: 1rem;
		padding: 0.2rem 0;
	}
	li strong {
		white-space: nowrap;
		font-family: var(--font-mono);
	}
	.negative {
		color: var(--error, #fb7185);
	}
	.legacy-score {
		margin-top: 0.5rem;
	}
</style>
