import { writable } from 'svelte/store';

export interface AISession {
	id: number;
	title: string;
	created_at: string;
	updated_at: string;
}

export interface AIMessage {
	id?: number;
	role: 'system' | 'user' | 'assistant' | 'tool';
	content: string;
	tool_calls?: string;
	tool_call_id?: string;
	streaming?: boolean;
}

export interface AIRecommendation {
	mal_id?: number;
	title: string;
	media_type: string;
	reason: string;
	poster_url?: string;
	score?: number;
}

export const sessions = writable<AISession[]>([]);
export const currentSessionId = writable<number | null>(null);
export const messages = writable<AIMessage[]>([]);
export const recommendations = writable<AIRecommendation[]>([]);
export const isStreaming = writable(false);
export const toolStatus = writable<string | null>(null);
