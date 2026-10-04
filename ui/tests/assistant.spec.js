import { test, expect } from '@playwright/test';
import { mockApi } from './fixtures.mjs';
import { consumeSSE } from '../src/lib/sse';

for (const restored of [false, true]) {
	test(`assistant resolves cached movie and series covers in ${restored ? 'saved' : 'streamed'} recommendations`, async ({
		page
	}) => {
		await mockApi(page);
		const recommendations = [
			{ title: 'Repo Man (1984)', media_type: 'movie', reason: 'A cult classic.' },
			{ title: 'Dark (2017)', media_type: 'series', reason: 'An intricate mystery.' }
		];
		const searches = [];
		await page.route('**/api/search?**', (route) => {
			const params = new URL(route.request().url()).searchParams;
			searches.push([params.get('q'), params.get('type')]);
			const movie = params.get('type') === 'movie';
			const entry = {
				tmdb_id: movie ? 138 : 70523,
				title: movie ? 'Repo Man' : 'Dark',
				type: movie ? 'movie' : 'series',
				year: movie ? 1984 : 2017,
				poster_url: `https://image.tmdb.org/t/p/w500/${movie ? 'repo-man' : 'dark'}.jpg`
			};
			return route.fulfill({
				json: [
					{
						...entry,
						tmdb_id: 999,
						year: 2024,
						poster_url: 'https://image.tmdb.org/t/p/w500/wrong-remake.jpg'
					},
					entry
				]
			});
		});
		if (restored) {
			await page.route('**/api/ai/sessions', (route) =>
				route.fulfill({
					json: [{ id: 1, title: 'Cult picks', updated_at: new Date().toISOString() }]
				})
			);
			await page.route('**/api/ai/sessions/1', (route) =>
				route.fulfill({
					json: {
						messages: [
							{
								role: 'assistant',
								content: 'Try these.',
								tool_calls: JSON.stringify([
									{
										function: {
											name: 'show_recommendations',
											arguments: JSON.stringify({ recommendations })
										}
									}
								])
							}
						]
					}
				})
			);
		} else {
			await page.route('**/api/ai/sessions/1/messages', (route) =>
				route.fulfill({
					contentType: 'text/event-stream',
					body: `event: recommendations\ndata: ${JSON.stringify({ recommendations })}\n\nevent: done\ndata: {}\n\n`
				})
			);
		}
		await page.goto('/assistant');
		if (restored) {
			await page.getByRole('button', { name: 'Cult picks', exact: true }).click();
		} else {
			await page.getByLabel('Message the assistant').fill('Recommend a cult film');
			await page.getByRole('button', { name: 'Send', exact: true }).click();
		}
		for (const [title, file] of [
			['Repo Man (1984)', 'repo-man'],
			['Dark (2017)', 'dark']
		]) {
			const image = page.getByRole('img', { name: title, exact: true });
			await expect(image).toHaveAttribute(
				'src',
				`/api/image?url=${encodeURIComponent(`https://image.tmdb.org/t/p/w500/${file}.jpg`)}`
			);
			await expect
				.poll(() => image.evaluate((node) => node.complete && node.naturalWidth > 0))
				.toBe(true);
		}
		expect(searches).toEqual(
			expect.arrayContaining([
				['Repo Man', 'movie'],
				['Dark', 'series']
			])
		);
		await page
			.locator('.card')
			.filter({ hasText: 'Repo Man (1984)' })
			.getByRole('button', { name: '+ Add to Library', exact: true })
			.click();
		await expect(
			page.getByRole('dialog').getByRole('heading', { name: 'Repo Man', exact: true })
		).toBeVisible();
		await expect(page.locator('.detail[role="dialog"]')).toContainText('1984');
		expect(searches).toHaveLength(2);
	});
}

test('assistant keeps missing and failed covers usable on mobile', async ({ page }) => {
	await page.setViewportSize({ width: 390, height: 844 });
	await mockApi(page);
	const recommendations = [
		{ title: 'Unmatched (1984)', media_type: 'movie', reason: 'No matching year.' },
		{ title: 'Offline', media_type: 'series', reason: 'Metadata is offline.' },
		{ title: 'No Cover', media_type: 'movie', reason: 'Artwork is unavailable.' }
	];
	await page.route('**/api/search?**', (route) => {
		const title = new URL(route.request().url()).searchParams.get('q');
		if (title === 'Offline')
			return route.fulfill({ status: 503, json: { error: 'Metadata offline' } });
		return route.fulfill({
			json: [
				{
					title,
					type: 'movie',
					year: 2024,
					poster_url: 'https://image.tmdb.org/t/p/w500/unavailable.jpg'
				}
			]
		});
	});
	await page.route('**/api/image?**', (route) => route.fulfill({ status: 404 }));
	await page.route('**/api/ai/sessions/1/messages', (route) =>
		route.fulfill({
			contentType: 'text/event-stream',
			body: `event: recommendations\ndata: ${JSON.stringify({ recommendations })}\n\nevent: done\ndata: {}\n\n`
		})
	);
	await page.goto('/assistant');
	await page.getByLabel('Message the assistant').fill('Recommend something');
	const failedCover = page.waitForResponse(
		(response) => response.url().includes('/api/image?') && response.status() === 404
	);
	await page.getByRole('button', { name: 'Send', exact: true }).click();
	await failedCover;
	await expect(page.locator('.card .artwork-placeholder')).toHaveCount(3);
	await expect(page.locator('.card img')).toHaveCount(0);
	await expect(page.getByRole('button', { name: '+ Add to Library', exact: true })).toHaveCount(3);
	expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

test('SSE parser handles byte-split UTF-8 and multiline data', async () => {
	const bytes = new TextEncoder().encode(
		'event: text\r\ndata: {\r\ndata: "content":"Привет 🎵"}\r\n\r\nevent: done\ndata: {}'
	);
	const stream = new ReadableStream({
		start(controller) {
			for (const byte of bytes) controller.enqueue(new Uint8Array([byte]));
			controller.close();
		}
	});
	const events = [];
	await consumeSSE(stream, (event, data) => events.push({ event, data }));
	expect(events).toEqual([
		{ event: 'text', data: { content: 'Привет 🎵' } },
		{ event: 'done', data: {} }
	]);
});

test('assistant surfaces interrupted streams without losing partial answer', async ({ page }) => {
	await mockApi(page);
	await page.route('**/api/ai/sessions/1/messages', (route) =>
		route.fulfill({
			contentType: 'text/event-stream',
			body: 'event: text\ndata: {"content":"Partial answer"}\n\n'
		})
	);
	await page.goto('/assistant');
	await page.getByLabel('Message the assistant').fill('Hello');
	await page.getByRole('button', { name: 'Send', exact: true }).click();
	await expect(page.locator('.prose')).toHaveText('Partial answer');
	await expect(
		page.getByText('The connection ended before the assistant finished. Try again.', {
			exact: true
		})
	).toBeVisible();
	await expect(page.getByLabel('Message the assistant')).toBeEnabled();
});

test('assistant separates reasoning and parses CRLF and final unterminated events', async ({
	page
}) => {
	await mockApi(page);
	await page.route('**/api/ai/sessions/1/messages', (route) =>
		route.fulfill({
			contentType: 'text/event-stream',
			body: 'event: reasoning\r\ndata: {"content":"Considering the request."}\r\n\r\nevent:text\r\ndata:{"content":"Try Arrival."}\r\n\r\nevent: done\r\ndata: {}'
		})
	);
	await page.goto('/assistant');
	await page.getByLabel('Message the assistant').fill('What should I watch?');
	await page.getByRole('button', { name: 'Send', exact: true }).click();
	await expect(page.locator('.prose')).toHaveText('Try Arrival.');
	await expect(page.getByText('Model reasoning', { exact: true })).toBeVisible();
	await expect(page.getByText('Considering the request.', { exact: true })).not.toBeVisible();
	await page.getByText('Model reasoning', { exact: true }).click();
	await expect(page.getByText('Considering the request.', { exact: true })).toBeVisible();
	await expect(page.getByRole('button', { name: 'Send', exact: true })).toBeVisible();
});

test('assistant prevents duplicate submissions while creating a session', async ({ page }) => {
	await mockApi(page);
	let count = 0;
	let release;
	const gate = new Promise((resolve) => {
		release = resolve;
	});
	await page.route('**/api/ai/sessions', async (route) => {
		if (route.request().method() !== 'POST') return route.fulfill({ json: [] });
		count++;
		await gate;
		await route.fulfill({ json: { id: 1 } });
	});
	await page.route('**/api/ai/sessions/1/messages', (route) =>
		route.fulfill({
			contentType: 'text/event-stream',
			body: 'event: text\ndata: {"content":"Hello."}\n\nevent: done\ndata: {}\n\n'
		})
	);
	await page.goto('/assistant');
	await page.getByLabel('Message the assistant').fill('Hello');
	await page.getByLabel('Message the assistant').press('Enter');
	await expect.poll(() => count).toBe(1);
	await expect(page.getByLabel('Message the assistant')).toBeDisabled();
	await expect(page.getByRole('button', { name: '+ New Chat' })).toBeDisabled();
	release();
	await expect(page.locator('.prose')).toHaveText('Hello.');
	expect(count).toBe(1);
});

test('late conversation response cannot overwrite the selected chat', async ({ page }) => {
	await mockApi(page);
	await page.route('**/api/ai/sessions', (route) =>
		route.fulfill({
			json: [
				{ id: 1, title: 'First chat', updated_at: '2026-10-03T12:00:00Z' },
				{ id: 2, title: 'Second chat', updated_at: '2026-10-03T12:00:00Z' }
			]
		})
	);
	let release;
	const gate = new Promise((resolve) => {
		release = resolve;
	});
	await page.route('**/api/ai/sessions/1', async (route) => {
		await gate;
		await route.fulfill({ json: { messages: [{ role: 'user', content: 'Stale message' }] } });
	});
	await page.route('**/api/ai/sessions/2', (route) =>
		route.fulfill({ json: { messages: [{ role: 'user', content: 'Current message' }] } })
	);
	await page.goto('/assistant');
	await page.getByRole('button', { name: 'First chat', exact: true }).click();
	await page.getByRole('button', { name: 'Second chat', exact: true }).click();
	await expect(page.getByText('Current message', { exact: true })).toBeVisible();
	release();
	await page.waitForTimeout(100);
	await expect(page.getByText('Stale message', { exact: true })).not.toBeVisible();
});
