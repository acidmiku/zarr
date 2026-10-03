import { test, expect } from '@playwright/test';
import { mockApi } from './fixtures.mjs';
import { consumeSSE } from '../src/lib/sse';

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
