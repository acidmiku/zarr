/** Read complete SSE frames even when the network splits UTF-8, CRLF, or the final frame. */
export async function consumeSSE(
	body: ReadableStream<Uint8Array>,
	onEvent: (event: string, data: any) => void
) {
	const reader = body.getReader();
	const decoder = new TextDecoder();
	let buffer = '';
	let finished = false;
	function dispatch(frame: string) {
		let event = 'message';
		const data: string[] = [];
		for (const line of frame.split(/\r?\n/)) {
			if (line.startsWith('event:')) event = line.slice(6).trim();
			if (line.startsWith('data:')) data.push(line.slice(5).replace(/^ /, ''));
		}
		if (!data.length || data.join('\n') === '[DONE]') return;
		onEvent(event, JSON.parse(data.join('\n')));
	}
	try {
		while (true) {
			const { done, value } = await reader.read();
			buffer += done ? decoder.decode() : decoder.decode(value, { stream: true });
			let match;
			while ((match = /\r?\n\r?\n/.exec(buffer))) {
				dispatch(buffer.slice(0, match.index));
				buffer = buffer.slice(match.index + match[0].length);
			}
			if (done) {
				finished = true;
				if (buffer.trim()) dispatch(buffer);
				break;
			}
		}
	} finally {
		if (!finished) await reader.cancel().catch(() => {});
		reader.releaseLock();
	}
}
