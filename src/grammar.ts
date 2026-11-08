import type { GrammarContent, GrammarGroup, GrammarRow } from '../shared/types';
import { HttpError, object, text } from './http';

export const GAP = '___';

const string = { type: 'string' }, strings = { type: 'array', items: string }, nullableString = { type: ['string', 'null'] };
const shape = (properties: Record<string, unknown>) => ({ type: 'object', properties, required: Object.keys(properties), additionalProperties: false });
export const grammarSchema = shape({ pl: string, examplePl: string, alternativesPl: strings, grammar: shape({
	used: string, right: string, answer: string, facts: strings, slot: strings, mistake: nullableString,
	table: shape({ title: string, columns: strings, groups: { type: 'array', items: shape({
		title: nullableString, rows: { type: 'array', items: shape({ names: strings, endings: string, examples: strings, hit: { type: 'boolean' } }) },
	}) } }),
	explanation: string,
}) });

function list(value: unknown, min: number, max: number): unknown[] {
	if (!Array.isArray(value) || value.length < min || value.length > max) throw new Error('Invalid list.');
	return value;
}

const texts = (value: unknown, min: number, max: number, length: number) => list(value, min, max).map(item => text(item, length).trim());
const nullableText = (value: unknown, length: number) => value === null ? null : text(value, length).trim();

function row(value: unknown): GrammarRow {
	const data = object(value, ['names', 'endings', 'examples', 'hit']);
	const names = texts(data.names, 1, 3, 40);
	if (typeof data.hit !== 'boolean') throw new Error('Invalid hit.');
	return { names, endings: text(data.endings, 60).trim(), examples: texts(data.examples, names.length, names.length, 80), hit: data.hit };
}

function group(value: unknown): GrammarGroup {
	const data = object(value, ['title', 'rows']);
	return { title: nullableText(data.title, 80), rows: list(data.rows, 1, 8).map(row) };
}

function parse(value: unknown): GrammarContent {
	const data = object(value, ['pl', 'examplePl', 'alternativesPl', 'grammar']);
	const grammar = object(data.grammar, ['used', 'right', 'answer', 'facts', 'slot', 'mistake', 'table', 'explanation']);
	const table = object(grammar.table, ['title', 'columns', 'groups']);
	const examplePl = text(data.examplePl, 500).trim();
	const groups = list(table.groups, 1, 4).map(group);
	const rows = groups.flatMap(item => item.rows);
	if (examplePl.split(GAP).length !== 2 || rows.length > 12 || rows.filter(item => item.hit).length !== 1) throw new Error('Invalid grammar card.');
	return {
		pl: text(data.pl, 200).trim(), examplePl, alternativesPl: texts(data.alternativesPl, 0, 4, 200),
		grammar: {
			used: text(grammar.used, 200).trim(), right: text(grammar.right, 200).trim(), answer: text(grammar.answer, 200).trim(),
			facts: texts(grammar.facts, 1, 4, 40), slot: texts(grammar.slot, 1, 5, 40), mistake: nullableText(grammar.mistake, 60),
			table: { title: text(table.title, 100).trim(), columns: texts(table.columns, 3, 3, 40) as [string, string, string], groups },
			explanation: text(grammar.explanation, 500).trim(),
		},
	};
}

export function parseGrammarContent(value: unknown): GrammarContent {
	try { return parse(value); }
	catch { throw new HttpError(400, 'invalid_input', 'Sprawdź pola fiszki gramatycznej.'); }
}
