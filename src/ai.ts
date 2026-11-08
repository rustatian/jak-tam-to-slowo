import Anthropic from '@anthropic-ai/sdk';
import type { Env } from './index';
import { HttpError } from './http';

const unconfigured = () => new HttpError(503, 'ai_unavailable', 'Model AI nie jest skonfigurowany. Poproś właściciela o sprawdzenie klucza API.');
const invalid = () => new HttpError(503, 'ai_invalid_response', 'Model AI zwrócił nieprawidłową odpowiedź. Spróbuj ponownie.');

export interface AiOptions { model: string; effort: 'high' | 'xhigh'; thinking?: { type: 'adaptive' }; timeout: number }
export const defaultAi: AiOptions = { model: 'claude-sonnet-5-5', effort: 'high', timeout: 45_000 };
export const grammarAi: AiOptions = { model: 'claude-opus-5-5', effort: 'xhigh', thinking: { type: 'adaptive' }, timeout: 90_000 };

export async function requestAiJson(env: Env, instructions: string, input: unknown, schema: Record<string, unknown>, options: AiOptions = defaultAi): Promise<unknown> {
	const signal = AbortSignal.timeout(options.timeout);
	let requestId: string | null | undefined;
	try {
		if (!env.ANTHROPIC_API_KEY) throw unconfigured();
		const client = new Anthropic({ apiKey: env.ANTHROPIC_API_KEY, timeout: options.timeout, maxRetries: 0 });
		const message = await client.messages.create({
			model: options.model, max_tokens: 16_000, system: instructions, messages: [{ role: 'user', content: JSON.stringify(input) }],
			...options.thinking && { thinking: options.thinking },
			output_config: { effort: options.effort, format: { type: 'json_schema', schema } },
		}, { signal });
		requestId = message._request_id;
		if (message.stop_reason === 'refusal') throw new HttpError(503, 'ai_refused', 'Model AI odmówił odpowiedzi. Zmień zapytanie.');
		if (message.stop_reason !== 'end_turn') throw invalid();
		// Thinking blocks can come before the text.
		try { return JSON.parse(message.content.map(block => block.type === 'text' ? block.text : '').join('')); } catch { throw invalid(); }
	} catch (cause) {
		// handleReview marks only an HttpError as editable, so every failure becomes one.
		const error = cause instanceof HttpError ? cause
			: signal.aborted || cause instanceof Anthropic.APIConnectionTimeoutError ? new HttpError(503, 'ai_timeout', 'Model AI nie odpowiedział na czas. Spróbuj ponownie.')
			: cause instanceof Anthropic.AuthenticationError || cause instanceof Anthropic.PermissionDeniedError ? unconfigured()
			: cause instanceof Anthropic.RateLimitError ? new HttpError(429, 'ai_limit', 'Osiągnięto limit modelu AI. Spróbuj później.')
			: new HttpError(503, 'ai_unavailable', 'Model AI jest teraz niedostępny. Spróbuj ponownie.');
		if (cause instanceof Anthropic.APIError) requestId = cause.requestID;
		const providerStatus = cause instanceof Anthropic.APIError ? cause.status ?? null : null;
		console.error(JSON.stringify({ event: 'ai_failed', status: error.status, code: error.code, providerStatus, requestId: requestId ?? null }));
		throw error;
	}
}
