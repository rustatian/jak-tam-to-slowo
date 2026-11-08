import { expect, it } from 'vitest';
import type { GrammarContent } from '../shared/types';
import { parseGrammarContent } from '../src/grammar';
import { GRAMMAR_DRAFT } from './fixtures';

type Draft = { pl: string; examplePl: string; alternativesPl: string[]; grammar: { used: string; right: string; answer: string; facts: string[]; slot: string[]; mistake: string | null; explanation: string;
	table: { title: string; columns: string[]; groups: { title: string | null; rows: { names: string[]; endings: string; examples: string[]; hit: boolean }[] }[] } } };
const draft = (change: (card: Draft) => unknown) => change(structuredClone(GRAMMAR_DRAFT) as Draft);
const rows = (length: number) => Array.from({ length }, () => ({ names: ['x'], endings: '-x', examples: ['x'], hit: false }));
function failure(value: unknown): unknown {
	try { parseGrammarContent(value); return null; } catch (error) { return error; }
}

it('accepts the fixture and trims its strings', () => {
	const padded = structuredClone(GRAMMAR_DRAFT);
	padded.pl = ' pies ';
	padded.grammar.answer = ' psy ';
	expect(parseGrammarContent(padded)).toEqual(GRAMMAR_DRAFT);
});

it.each<{ name: string; change: (card: Draft) => unknown }>([
	{ name: 'a sentence without a gap', change: c => ({ ...c, examplePl: 'Na podwórku biegają dwa psy.' }) },
	{ name: 'a sentence with two gaps', change: c => ({ ...c, examplePl: '___ biegają dwa ___.' }) },
	{ name: 'no hit row', change: c => { c.grammar.table.groups[1].rows[0].hit = false; return c; } },
	{ name: 'two hit rows', change: c => { c.grammar.table.groups[0].rows[0].hit = true; return c; } },
	{ name: 'fewer examples than names', change: c => { c.grammar.table.groups[1].rows[0].examples = ['pies → psy']; return c; } },
	{ name: 'five word facts', change: c => { c.grammar.facts = ['a', 'b', 'c', 'd', 'e']; return c; } },
	{ name: 'six slot labels', change: c => { c.grammar.slot = ['a', 'b', 'c', 'd', 'e', 'f']; return c; } },
	{ name: 'a label over 40 characters', change: c => { c.grammar.slot[0] = 'a'.repeat(41); return c; } },
	{ name: 'five accepted variants', change: c => ({ ...c, alternativesPl: ['a', 'b', 'c', 'd', 'e'] }) },
	{ name: 'a mistake over 60 characters', change: c => { c.grammar.mistake = 'a'.repeat(61); return c; } },
	{ name: 'two columns', change: c => { c.grammar.table.columns = ['Rodzaj', 'Końcówka']; return c; } },
	{ name: 'thirteen rows', change: c => { c.grammar.table.groups[0].rows = rows(8); c.grammar.table.groups.push({ title: null, rows: rows(3) }); return c; } },
	{ name: 'nine rows in one group', change: c => { c.grammar.table.groups[0].rows = rows(9); return c; } },
	{ name: 'a blank answer', change: c => { c.grammar.answer = ' '; return c; } },
	{ name: 'an explanation over 500 characters', change: c => { c.grammar.explanation = 'a'.repeat(501); return c; } },
	{ name: 'an extra field', change: c => ({ ...c, ru: 'Собака' }) },
])('rejects $name', ({ change }) => {
	expect(failure(draft(change))).toMatchObject({ status: 400, code: 'invalid_input', message: 'Sprawdź pola fiszki gramatycznej.' });
});

it('keeps a null mistake for a correct used form', () => {
	const correct: GrammarContent = structuredClone(GRAMMAR_DRAFT);
	correct.grammar.used = 'dwa psy';
	correct.grammar.mistake = null;
	expect(parseGrammarContent(correct).grammar.mistake).toBeNull();
});
