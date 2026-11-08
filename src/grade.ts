import type { Card, Direction, GradeResult } from '../shared/types';
import type { Env } from './index';
import { requestAiJson } from './ai';
import { HttpError, object, text } from './http';
import grammarInstructions from './prompts/grade-grammar.txt';
import instructions from './prompts/grade.txt';

const schema = {
	type: 'object', properties: { grade: { type: 'string', enum: ['correct', 'almost', 'again'] }, explanation: { type: 'string' } },
	required: ['grade', 'explanation'], additionalProperties: false,
};

function normalized(value: string): string {
	return value.normalize('NFC').toLowerCase().replace(/ё/g, 'е').trim().replace(/\s+/g, ' ');
}

// Only for Polish: NFD splits Russian й into и plus a combining mark.
function withoutPolishMarks(value: string): string {
	return value.normalize('NFD').replace(/\p{M}/gu, '').replace(/ł/g, 'l');
}

export function correctionOf(card: Card, direction: Direction): string {
	if (card.grammar) return card.grammar.answer;
	return direction === 'ru-pl' ? card.pl : card.ru;
}

export async function gradeAnswer(env: Env, card: Card, direction: Direction, answer: string): Promise<{ result: GradeResult; source: 'exact' | 'ai' }> {
	const { grammar } = card;
	const correction = correctionOf(card, direction);
	// A grammar card asks for a Polish form in both directions.
	const polish = grammar !== null || direction === 'ru-pl';
	const alternatives = polish ? card.alternativesPl : card.alternativesRu;
	const value = normalized(answer);
	if (!value) return { result: { grade: 'again', correction, explanation: 'Brak odpowiedzi. Powtórz słowo.' }, source: 'exact' };
	const terms = [correction, ...alternatives];
	if (terms.some(term => normalized(term) === value)) {
		return { result: { grade: 'correct', correction, explanation: 'Dobrze.' }, source: 'exact' };
	}
	const near = polish && terms.find(term => withoutPolishMarks(normalized(term)) === withoutPolishMarks(value));
	if (near) return { result: { grade: 'almost', correction, explanation: `Prawie dobrze. Sprawdź polskie litery: ${near}.` }, source: 'exact' };
	const { pl, ru, examplePl, exampleRu, meaningRu, alternativesPl, alternativesRu } = card;
	const response = grammar
		? await requestAiJson(env, grammarInstructions, { kind: 'grammar', card: { pl, examplePl, answer: grammar.answer, alternativesPl, facts: grammar.facts, slot: grammar.slot }, answer }, schema)
		: await requestAiJson(env, instructions, { card: { pl, ru, examplePl, exampleRu, meaningRu, alternativesPl, alternativesRu }, direction, answer }, schema);
	try {
		const data = object(response, ['grade', 'explanation']);
		if (data.grade !== 'correct' && data.grade !== 'almost' && data.grade !== 'again') throw new Error('Invalid grade.');
		return { result: { grade: data.grade, correction, explanation: text(data.explanation, 500).trim() }, source: 'ai' };
	} catch { throw new HttpError(503, 'ai_invalid_response', 'Model AI zwrócił nieprawidłową ocenę. Spróbuj ponownie.'); }
}
