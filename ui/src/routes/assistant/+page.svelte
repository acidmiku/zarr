<script>
	import { onMount, tick } from 'svelte';
	import { api } from '$lib/api';
	import { notify } from '$lib/stores/app';
	import RecommendationCard from '$lib/components/RecommendationCard.svelte';
	import ToolIndicator from '$lib/components/ToolIndicator.svelte';
	import MediaDetail from '$lib/components/MediaDetail.svelte';
	import MusicDetail from '$lib/components/MusicDetail.svelte';

	let sessions = [];
	let currentSessionId = null;
	let messages = [];
	let isStreaming = false;
	let toolStatus = null;
	let inputText = '';
	let chatContainer;
	let settings = null;
	let loading = true;
	let profiles = [];
	let selectedItem = null;
	let musicDetailItem = null;

	onMount(async () => {
		try {
			settings = await api.getSettings();
			profiles = await api.getProfiles();
			if (settings.openrouter_configured) {
				await loadSessions();
			}
		} catch (e) {
			notify(e.message, 'error');
		}
		loading = false;
	});

	async function loadSessions() {
		try {
			sessions = await api.aiSessions();
		} catch (e) {
			sessions = [];
		}
	}

	async function selectSession(id) {
		if (isStreaming) return;
		currentSessionId = id;
		try {
			const data = await api.aiGetSession(id);
			messages = (data.messages || [])
				.filter(m => m.role === 'user' || m.role === 'assistant')
				.map(m => ({ ...m, recommendations: extractRecommendations(m) }))
				.filter(m => m.role === 'user' || m.content?.trim() || m.recommendations.length > 0);
			await scrollToBottom();
		} catch (e) {
			notify(e.message, 'error');
		}
	}

	function extractRecommendations(msg) {
		if (msg.role !== 'assistant' || !msg.tool_calls) return [];
		try {
			const tcs = JSON.parse(msg.tool_calls);
			for (const tc of tcs) {
				if (tc.function?.name === 'show_recommendations') {
					const args = JSON.parse(tc.function.arguments);
					return (args.recommendations || []).map(r => {
						let poster_url = null;
						if (r.media_type === 'music' && r.release_group_id) {
							poster_url = `/api/music/cover?rgid=${r.release_group_id}`;
						} else if (r.mal_id) {
							poster_url = `/api/image/jikan/${r.mal_id}`;
						}
						return {
							title: r.title,
							media_type: r.media_type,
							reason: r.reason,
							poster_url,
							score: r.score || null,
							release_group_id: r.release_group_id || null
						};
					});
				}
			}
		} catch {}
		return [];
	}

	async function newChat() {
		if (isStreaming) return;
		currentSessionId = null;
		messages = [];
	}

	async function deleteSession(id) {
		if (!confirm('Delete this conversation?')) return;
		try {
			await api.aiDeleteSession(id);
			if (currentSessionId === id) {
				currentSessionId = null;
				messages = [];
			}
			await loadSessions();
		} catch (e) {
			notify(e.message, 'error');
		}
	}

	async function sendMessage() {
		if (!inputText.trim() || isStreaming) return;

		let sessionId = currentSessionId;
		if (!sessionId) {
			try {
				const session = await api.aiCreateSession();
				sessionId = session.id;
				currentSessionId = sessionId;
				await loadSessions();
			} catch (e) {
				notify(e.message, 'error');
				return;
			}
		}

		const text = inputText.trim();
		inputText = '';
		messages = [...messages, { role: 'user', content: text, recommendations: [] }];
		messages = [...messages, { role: 'assistant', content: '', streaming: true, recommendations: [] }];
		await scrollToBottom();

		await streamResponse(api.aiChat(sessionId, text));
	}

	async function getRecommendations() {
		let sessionId = currentSessionId;
		if (!sessionId) {
			try {
				const session = await api.aiCreateSession();
				sessionId = session.id;
				currentSessionId = sessionId;
				await loadSessions();
			} catch (e) {
				notify(e.message, 'error');
				return;
			}
		}

		messages = [...messages, { role: 'assistant', content: '', streaming: true, recommendations: [] }];
		await scrollToBottom();

		await streamResponse(api.aiRecommend(sessionId));
	}

	async function streamResponse(responsePromise) {
		isStreaming = true;
		toolStatus = null;

		try {
			const response = await responsePromise;
			if (!response.ok) {
				const err = await response.json().catch(() => ({ error: 'Request failed' }));
				throw new Error(err.error || 'Request failed');
			}

			const reader = response.body.getReader();
			const decoder = new TextDecoder();
			let buffer = '';
			let currentEventType = '';

			while (true) {
				const { done, value } = await reader.read();
				if (done) break;

				buffer += decoder.decode(value, { stream: true });
				const lines = buffer.split('\n');
				buffer = lines.pop() || '';

				for (const line of lines) {
					if (line.startsWith('event: ')) {
						currentEventType = line.slice(7).trim();
						continue;
					}
					if (line.startsWith('data: ')) {
						const dataStr = line.slice(6);
						try {
							const data = JSON.parse(dataStr);
							handleSSEEvent(currentEventType, data);
						} catch {}
						currentEventType = '';
					}
				}
			}
		} catch (e) {
			notify(e.message, 'error');
			messages = messages.filter(m => !(m.streaming && m.content === '' && m.recommendations.length === 0));
		}

		isStreaming = false;
		toolStatus = null;
		messages = messages.map(m => m.streaming ? { ...m, streaming: false } : m);
		await loadSessions();
	}

	function handleSSEEvent(eventType, data) {
		switch (eventType) {
			case 'text':
				appendToCurrentMessage(data.content);
				scrollToBottom();
				break;
			case 'tool_call_start':
				toolStatus = data.tool;
				break;
			case 'tool_call_end':
				toolStatus = null;
				break;
			case 'recommendations':
				attachRecommendations(data.recommendations);
				scrollToBottom();
				break;
			case 'new_message':
				// Tool loop continuing — create new assistant bubble
				messages = [...messages, { role: 'assistant', content: '', streaming: true, recommendations: [] }];
				scrollToBottom();
				break;
			case 'error':
				notify(data.message, 'error');
				break;
		}
	}

	function appendToCurrentMessage(text) {
		const lastIdx = messages.length - 1;
		if (lastIdx >= 0 && messages[lastIdx].role === 'assistant') {
			messages[lastIdx] = { ...messages[lastIdx], content: messages[lastIdx].content + text };
			messages = messages;
		}
	}

	function attachRecommendations(recs) {
		const lastIdx = messages.length - 1;
		if (lastIdx >= 0 && messages[lastIdx].role === 'assistant') {
			messages[lastIdx] = { ...messages[lastIdx], recommendations: [...(messages[lastIdx].recommendations || []), ...recs] };
			messages = messages;
		}
	}

	async function scrollToBottom() {
		await tick();
		if (chatContainer) {
			chatContainer.scrollTop = chatContainer.scrollHeight;
		}
	}

	function handleKeydown(e) {
		if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault();
			sendMessage();
		}
	}

	function groupSessions(sessions) {
		const now = new Date();
		const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
		const yesterday = new Date(today.getTime() - 86400000);
		const groups = { today: [], yesterday: [], older: [] };
		for (const s of sessions) {
			const d = new Date(s.updated_at);
			if (d >= today) groups.today.push(s);
			else if (d >= yesterday) groups.yesterday.push(s);
			else groups.older.push(s);
		}
		return groups;
	}

	function fmtInline(text) {
		return text
			.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
			.replace(/\*(.+?)\*/g, '<em>$1</em>')
			.replace(/`(.+?)`/g, '<code>$1</code>');
	}

	function formatMarkdown(text) {
		if (!text) return '';
		text = text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

		const lines = text.split('\n');
		const blocks = [];
		let cur = { type: 'text', lines: [] };

		for (const line of lines) {
			const trimmed = line.trim();

			if (trimmed === '') {
				if (cur.lines.length > 0) { blocks.push(cur); }
				cur = { type: 'text', lines: [] };
				continue;
			}

			const isHeader = /^#{2,4} /.test(trimmed);
			const isBullet = /^[-*] .+/.test(trimmed);
			const isOrdered = /^\d+\. .+/.test(trimmed);

			if (isHeader) {
				if (cur.lines.length > 0) blocks.push(cur);
				blocks.push({ type: 'header', lines: [trimmed] });
				cur = { type: 'text', lines: [] };
			} else if (isBullet || isOrdered) {
				if (cur.type !== 'list') {
					if (cur.lines.length > 0) blocks.push(cur);
					cur = { type: 'list', lines: [] };
				}
				cur.lines.push(trimmed);
			} else {
				if (cur.type !== 'text') {
					if (cur.lines.length > 0) blocks.push(cur);
					cur = { type: 'text', lines: [] };
				}
				cur.lines.push(trimmed);
			}
		}
		if (cur.lines.length > 0) blocks.push(cur);

		return blocks.map(block => {
			if (block.type === 'header') {
				const l = block.lines[0];
				const m3 = l.match(/^## (.+)/);
				if (m3) return `<h3>${fmtInline(m3[1])}</h3>`;
				const m4 = l.match(/^###+ (.+)/);
				if (m4) return `<h4>${fmtInline(m4[1])}</h4>`;
				return `<h4>${fmtInline(l.replace(/^#+\s*/, ''))}</h4>`;
			}
			if (block.type === 'list') {
				const items = block.lines.map(l => {
					const b = l.match(/^[-*] (.+)/);
					if (b) return `<li>${fmtInline(b[1])}</li>`;
					const o = l.match(/^\d+\. (.+)/);
					if (o) return `<li class="ol-item">${fmtInline(o[1])}</li>`;
					return `<li>${fmtInline(l)}</li>`;
				});
				return `<ul>${items.join('')}</ul>`;
			}
			return `<p>${fmtInline(block.lines.join('<br>'))}</p>`;
		}).join('');
	}

	function handleShowDetail(event) {
		selectedItem = event.detail;
	}

	function handleShowMusicDetail(event) {
		musicDetailItem = event.detail;
	}

	function handleAdded() {
		notify('Added to library!', 'success');
		selectedItem = null;
		musicDetailItem = null;
	}

	$: grouped = groupSessions(sessions);
</script>

<svelte:head>
	<title>Assistant - Zarr</title>
</svelte:head>

<div class="page">
	{#if loading}
		<div class="loading">Loading...</div>
	{:else if !settings?.openrouter_configured}
		<div class="setup-prompt">
			<div class="setup-card">
				<h2>AI Assistant</h2>
				<p>Configure an OpenRouter API key in Settings to enable the AI recommendation assistant.</p>
				<a href="/settings" class="btn btn-primary">Go to Settings</a>
			</div>
		</div>
	{:else}
		<div class="assistant-layout">
			<!-- Sidebar -->
			<aside class="sidebar">
				<button class="new-chat-btn" on:click={newChat}>+ New Chat</button>

				{#each [{ label: 'Today', items: grouped.today }, { label: 'Yesterday', items: grouped.yesterday }, { label: 'Older', items: grouped.older }] as group}
					{#if group.items.length}
						<div class="group-label">{group.label}</div>
						{#each group.items as s}
							<button
								class="session-item"
								class:active={currentSessionId === s.id}
								on:click={() => selectSession(s.id)}
							>
								<span class="session-title">{s.title || 'New conversation'}</span>
								<button class="delete-btn" on:click|stopPropagation={() => deleteSession(s.id)}>x</button>
							</button>
						{/each}
					{/if}
				{/each}
			</aside>

			<!-- Main chat area -->
			<div class="chat-area">
				<div class="messages" bind:this={chatContainer}>
					{#if messages.length === 0 && !currentSessionId}
						<div class="welcome">
							<div class="welcome-card">
								<h2>What should you watch or listen to next?</h2>
								<p>I'll analyze your ratings and suggest personalized recommendations, or we can just chat about movies, series, anime, and music.</p>
								<button class="btn btn-primary" on:click={getRecommendations}>
									Get Recommendations
								</button>
								<p class="welcome-sub">Or just start chatting below.</p>
							</div>
						</div>
					{/if}

					{#each messages as msg, i}
						{#if msg.role === 'user'}
							<div class="msg msg-user">
								<div class="msg-bubble user-bubble">{msg.content}</div>
							</div>
						{:else if msg.role === 'assistant'}
							<div class="msg msg-assistant">
								<div class="msg-bubble assistant-bubble">
									{#if msg.content}
										<div class="prose">{@html formatMarkdown(msg.content)}</div>
									{/if}
									{#if msg.streaming && !msg.content && (!msg.recommendations || msg.recommendations.length === 0)}
										<span class="typing-dot"></span>
									{/if}
									{#if msg.recommendations && msg.recommendations.length > 0}
										<div class="recs-inline">
											{#each msg.recommendations as rec}
												<RecommendationCard {rec} on:showDetail={handleShowDetail} on:showMusicDetail={handleShowMusicDetail} />
											{/each}
										</div>
									{/if}
								</div>
							</div>
						{/if}
					{/each}

					{#if toolStatus}
						<div class="msg msg-assistant">
							<ToolIndicator tool={toolStatus} />
						</div>
					{/if}
				</div>

				<!-- Input -->
				<div class="input-area">
					<textarea
						bind:value={inputText}
						on:keydown={handleKeydown}
						placeholder="Ask about movies, series, anime, music..."
						rows="1"
						disabled={isStreaming}
					></textarea>
					<button class="send-btn" on:click={sendMessage} disabled={isStreaming || !inputText.trim()}>
						{isStreaming ? '...' : 'Send'}
					</button>
				</div>
			</div>
		</div>
	{/if}
</div>

{#if selectedItem}
	<!-- svelte-ignore a11y-click-events-have-key-events -->
	<div class="modal-overlay" on:click={() => selectedItem = null} role="presentation">
		<div class="modal" on:click|stopPropagation on:keydown|stopPropagation role="dialog">
			<button class="modal-close" on:click={() => selectedItem = null}>✕</button>
			<MediaDetail
				item={selectedItem}
				{profiles}
				mode="discovery"
				on:added={handleAdded}
				on:error={(e) => notify(e.detail, 'error')}
			/>
		</div>
	</div>
{/if}

{#if musicDetailItem}
	<!-- svelte-ignore a11y-click-events-have-key-events -->
	<div class="modal-overlay" on:click={() => musicDetailItem = null} role="presentation">
		<div class="modal" on:click|stopPropagation on:keydown|stopPropagation role="dialog">
			<button class="modal-close" on:click={() => musicDetailItem = null}>✕</button>
			<MusicDetail
				item={musicDetailItem}
				{profiles}
				on:added={handleAdded}
				on:error={(e) => notify(e.detail, 'error')}
			/>
		</div>
	</div>
{/if}

<style>
	/* ── Keyframes ── */
	@keyframes fadeIn {
		from { opacity: 0; }
		to { opacity: 1; }
	}

	@keyframes slideUp {
		from { transform: translateY(20px); opacity: 0; }
		to { transform: translateY(0); opacity: 1; }
	}

	@keyframes fadeSlideUp {
		from { opacity: 0; transform: translateY(8px); }
		to { opacity: 1; transform: translateY(0); }
	}

	@keyframes pulse {
		0%, 100% { opacity: 0.3; }
		50% { opacity: 1; }
	}

	/* ── Modal ── */
	.modal-overlay {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.75);
		backdrop-filter: blur(6px);
		-webkit-backdrop-filter: blur(6px);
		z-index: 1000;
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 1rem;
		animation: fadeIn 0.15s ease;
	}

	.modal {
		position: relative;
		max-width: 900px;
		width: 100%;
		max-height: 90vh;
		overflow-y: auto;
		background: var(--glass-bg);
		backdrop-filter: blur(var(--glass-blur));
		-webkit-backdrop-filter: blur(var(--glass-blur));
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-lg);
		animation: slideUp 0.2s ease;
	}

	.modal-close {
		position: absolute;
		top: 1rem;
		right: 1rem;
		width: 32px;
		height: 32px;
		border-radius: 50%;
		border: 1px solid var(--border);
		background: var(--bg-elevated);
		color: var(--text-secondary);
		font-size: 1.2rem;
		display: flex;
		align-items: center;
		justify-content: center;
		cursor: pointer;
		z-index: 10;
		transition: all 0.2s ease;
	}
	.modal-close:hover {
		background: var(--bg-hover);
		color: var(--text-primary);
		border-color: var(--accent);
	}

	/* ── Page Shell ── */
	.page {
		height: calc(100vh - 3rem);
		display: flex;
		flex-direction: column;
		font-family: var(--font-body);
	}

	.loading, .setup-prompt {
		display: flex;
		align-items: center;
		justify-content: center;
		height: 100%;
		color: var(--text-muted);
		font-family: var(--font-body);
	}

	/* ── Setup Card ── */
	.setup-card {
		text-align: center;
		padding: 2.5rem;
		background: var(--glass-bg);
		backdrop-filter: blur(var(--glass-blur));
		-webkit-backdrop-filter: blur(var(--glass-blur));
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-lg);
		max-width: 400px;
	}
	.setup-card h2 {
		font-size: 1.2rem;
		margin-bottom: 0.5rem;
		color: var(--text-primary);
		font-family: var(--font-display);
		font-weight: 700;
		letter-spacing: -0.02em;
	}
	.setup-card p {
		color: var(--text-secondary);
		font-size: 0.85rem;
		margin-bottom: 1rem;
		font-family: var(--font-body);
	}

	/* ── Buttons ── */
	.btn {
		padding: 0.55rem 1.25rem;
		border-radius: var(--radius-md);
		font-size: 0.85rem;
		font-weight: 600;
		border: none;
		cursor: pointer;
		text-decoration: none;
		display: inline-block;
		font-family: var(--font-body);
		transition: all 0.25s ease;
	}
	.btn-primary {
		background: var(--accent);
		color: var(--text-inverse);
	}
	.btn-primary:hover {
		background: var(--accent-hover);
		box-shadow: 0 0 16px rgba(var(--accent-rgb, 99, 102, 241), 0.45);
	}

	/* ── Layout ── */
	.assistant-layout {
		display: flex;
		height: 100%;
	}

	/* ── Session Sidebar ── */
	.sidebar {
		width: 240px;
		min-width: 240px;
		background: var(--glass-bg);
		backdrop-filter: blur(var(--glass-blur));
		-webkit-backdrop-filter: blur(var(--glass-blur));
		border-right: 1px solid var(--glass-border);
		padding: 0.75rem;
		overflow-y: auto;
		display: flex;
		flex-direction: column;
		gap: 0.15rem;
	}
	.new-chat-btn {
		width: 100%;
		padding: 0.55rem;
		background: var(--bg-elevated);
		border: 1px solid var(--border);
		border-radius: var(--radius-md);
		color: var(--text-primary);
		font-size: 0.85rem;
		font-weight: 600;
		cursor: pointer;
		margin-bottom: 0.75rem;
		font-family: var(--font-body);
		transition: all 0.25s ease;
	}
	.new-chat-btn:hover {
		background: var(--bg-hover);
		border-color: var(--accent);
		color: var(--accent);
	}
	.group-label {
		font-size: 0.7rem;
		color: var(--text-muted);
		text-transform: uppercase;
		font-weight: 800;
		padding: 0.5rem 0.5rem 0.25rem;
		letter-spacing: -0.02em;
		font-family: var(--font-display);
	}
	.session-item {
		display: flex;
		align-items: center;
		justify-content: space-between;
		width: 100%;
		padding: 0.5rem 0.6rem;
		background: transparent;
		border: none;
		border-radius: var(--radius-md);
		color: var(--text-secondary);
		font-size: 0.8rem;
		cursor: pointer;
		text-align: left;
		font-family: var(--font-body);
		transition: background 0.25s ease, color 0.25s ease;
	}
	.session-item:hover {
		background: var(--bg-hover);
		color: var(--text-primary);
	}
	.session-item.active {
		background: var(--bg-active);
		color: var(--text-primary);
	}
	.session-title {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		flex: 1;
	}
	.delete-btn {
		opacity: 0;
		background: none;
		border: none;
		color: var(--text-muted);
		font-size: 0.85rem;
		cursor: pointer;
		padding: 0 0.25rem;
		flex-shrink: 0;
		transition: opacity 0.25s ease, color 0.2s ease;
	}
	.session-item:hover .delete-btn {
		opacity: 1;
	}
	.delete-btn:hover {
		color: var(--danger);
	}

	/* ── Chat Area ── */
	.chat-area {
		flex: 1;
		display: flex;
		flex-direction: column;
		min-width: 0;
	}

	.messages {
		flex: 1;
		overflow-y: auto;
		padding: 1.25rem 1.5rem;
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	/* ── Welcome State ── */
	.welcome {
		display: flex;
		align-items: center;
		justify-content: center;
		flex: 1;
		opacity: 0.85;
	}
	.welcome-card {
		text-align: center;
		padding: 2.5rem;
		max-width: 440px;
	}
	.welcome-card h2 {
		font-size: 1.3rem;
		margin-bottom: 0.65rem;
		color: var(--text-primary);
		font-family: var(--font-display);
		font-weight: 800;
		letter-spacing: -0.02em;
	}
	.welcome-card p {
		color: var(--text-secondary);
		font-size: 0.85rem;
		margin-bottom: 1.15rem;
		line-height: 1.5;
		font-family: var(--font-body);
	}
	.welcome-sub {
		font-size: 0.75rem !important;
		color: var(--text-muted) !important;
		margin-top: 0.5rem !important;
		opacity: 0.7;
	}

	/* ── Chat Messages ── */
	.msg {
		max-width: 80%;
		animation: fadeSlideUp 0.3s ease both;
	}
	.msg-user {
		align-self: flex-end;
	}
	.msg-assistant {
		align-self: flex-start;
	}

	.msg-bubble {
		padding: 0.75rem 1rem;
		font-size: 0.85rem;
		line-height: 1.55;
		font-family: var(--font-body);
		border-radius: var(--radius-md);
	}

	.user-bubble {
		background: var(--accent-bg);
		border: 1px solid var(--accent-subtle);
		border-radius: var(--radius-md) var(--radius-md) 4px var(--radius-md);
		white-space: pre-wrap;
		color: var(--text-primary);
	}

	.assistant-bubble {
		background: var(--glass-bg);
		backdrop-filter: blur(14px);
		-webkit-backdrop-filter: blur(14px);
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-md) var(--radius-md) var(--radius-md) 4px;
		color: var(--text-primary);
	}

	/* ── Prose (assistant markdown) ── */
	.prose :global(h3) {
		font-size: 0.95rem;
		font-weight: 700;
		color: var(--text-primary);
		margin: 0.65rem 0 0.25rem;
		font-family: var(--font-display);
		letter-spacing: -0.02em;
	}
	.prose :global(h3:first-child) { margin-top: 0; }
	.prose :global(h4) {
		font-size: 0.88rem;
		font-weight: 700;
		color: var(--accent);
		margin: 0.5rem 0 0.2rem;
		font-family: var(--font-display);
		letter-spacing: -0.02em;
	}
	.prose :global(h4:first-child) { margin-top: 0; }
	.prose :global(p) {
		margin: 0.3rem 0;
		line-height: 1.55;
		font-family: var(--font-body);
	}
	.prose :global(p:first-child) { margin-top: 0; }
	.prose :global(p:last-child) { margin-bottom: 0; }
	.prose :global(strong) {
		color: var(--accent);
		font-weight: 700;
	}
	.prose :global(em) {
		color: var(--text-secondary);
	}
	.prose :global(code) {
		background: var(--bg-inset);
		padding: 0.1rem 0.35rem;
		border-radius: 3px;
		font-size: 0.8rem;
	}
	.prose :global(ul) {
		margin: 0.3rem 0;
		padding-left: 1.1rem;
		list-style: none;
	}
	.prose :global(li) {
		position: relative;
		padding: 0.1rem 0;
		line-height: 1.5;
	}
	.prose :global(li)::before {
		content: '';
		position: absolute;
		left: -0.85rem;
		top: 0.55rem;
		width: 4px;
		height: 4px;
		border-radius: 50%;
		background: var(--accent);
	}
	.prose :global(li.ol-item) {
		list-style: decimal;
	}
	.prose :global(li.ol-item)::before {
		display: none;
	}

	/* ── Inline Recommendation Cards ── */
	.recs-inline {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
		margin-top: 0.5rem;
		padding-top: 0.5rem;
		border-top: 1px solid var(--glass-border);
	}

	/* ── Typing Indicator ── */
	.typing-dot {
		display: inline-block;
		width: 8px;
		height: 8px;
		background: var(--accent);
		border-radius: 50%;
		animation: pulse 1s ease-in-out infinite;
	}

	/* ── Input Area ── */
	.input-area {
		display: flex;
		gap: 0.5rem;
		padding: 0.75rem 1rem;
		border-top: 1px solid var(--glass-border);
		background: var(--glass-bg);
		backdrop-filter: blur(var(--glass-blur));
		-webkit-backdrop-filter: blur(var(--glass-blur));
	}
	textarea {
		flex: 1;
		padding: 0.6rem 0.85rem;
		background: var(--glass-bg);
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-md);
		color: var(--text-primary);
		font-size: 0.85rem;
		resize: none;
		outline: none;
		min-height: 38px;
		max-height: 120px;
		font-family: var(--font-body);
		transition: border-color 0.25s ease, box-shadow 0.25s ease;
	}
	textarea:focus {
		border-color: var(--accent);
		box-shadow: 0 0 0 3px rgba(var(--accent-rgb, 99, 102, 241), 0.2), 0 0 12px rgba(var(--accent-rgb, 99, 102, 241), 0.15);
	}
	.send-btn {
		padding: 0.6rem 1.25rem;
		background: var(--accent);
		border: none;
		border-radius: var(--radius-md);
		color: var(--text-inverse);
		font-size: 0.85rem;
		font-weight: 700;
		cursor: pointer;
		white-space: nowrap;
		font-family: var(--font-body);
		transition: all 0.25s ease;
	}
	.send-btn:hover {
		background: var(--accent-hover);
		box-shadow: 0 0 18px rgba(var(--accent-rgb, 99, 102, 241), 0.5);
	}
	.send-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
		box-shadow: none;
	}

	/* ── Responsive ── */
	@media (max-width: 768px) {
		.sidebar { display: none; }
		.msg { max-width: 95%; }
	}
</style>
