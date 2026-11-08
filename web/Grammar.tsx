import { Fragment, type ReactNode } from 'react';
import type { Grammar, GrammarContent } from '../shared/types';

type Token = { text: string; marked: boolean };

export function markDifferences(used: string, right: string): [Token[], Token[]] {
	const usedTokens = used.trim().split(/\s+/), rightTokens = right.trim().split(/\s+/);
	const same = usedTokens.length === rightTokens.length;
	const key = (token: string) => token.normalize('NFC').toLowerCase();
	const mark = (tokens: string[], other: string[]) => tokens.map((text, index) => ({ text, marked: !same || key(text) !== key(other[index]) }));
	return [mark(usedTokens, rightTokens), mark(rightTokens, usedTokens)];
}

function line(tokens: Token[], wrap: (text: string) => ReactNode) {
	return tokens.map((token, index) => <Fragment key={index}>{index > 0 && ' '}{token.marked ? wrap(token.text) : token.text}</Fragment>);
}

export function FormPair({ grammar }: { grammar: Grammar }): React.JSX.Element {
	const [used, right] = markDifferences(grammar.used, grammar.right);
	const wrong = grammar.mistake !== null;
	return <>
		<p className="kicker">{wrong ? 'Błędna forma' : 'Twoja forma'}</p>
		<p className="form-line">{line(used, text => wrong ? <s className="wrong">{text}</s> : text)}</p>
		<p className="kicker">Poprawnie</p>
		<p className="form-line">{line(right, text => <b className="right">{text}</b>)}</p>
	</>;
}

export function FilledSentence({ sentence, answer }: { sentence: string; answer: string }): React.JSX.Element {
	const [before, after] = sentence.split('___');
	return <>{before}<b className="right">{answer}</b>{after}</>;
}

export function GrammarDetails({ card, sentence }: { card: Pick<GrammarContent, 'grammar'>; sentence?: ReactNode }): React.JSX.Element {
	const { used, right, answer, facts, slot, mistake, table } = card.grammar;
	const tokens = markDifferences(used, right)[0];
	const struck = tokens.every(token => token.marked) ? used : tokens.find(token => token.marked)?.text ?? used;
	function example(text: string) {
		if (mistake === null) return text;
		if (!text.toLowerCase().endsWith(answer.toLowerCase())) return <>{text} (<s className="wrong">{used}</s>)</>;
		return <>{text.slice(0, text.length - answer.length)}<s className="wrong">{struck}</s> <b className="right">{answer}</b></>;
	}
	return <>
		<div className="labels">
			{facts.map((label, index) => <span key={`fact-${index}`} className="lab">{label}</span>)}
			{slot.map((label, index) => <span key={`slot-${index}`} className="lab hit">{label}</span>)}
			{mistake === null ? <span className="lab hit">forma poprawna</span> : <span className="lab err">błąd: {mistake}</span>}
		</div>
		{sentence}
		<p className="kicker">{table.title}</p>
		<table className="decl" aria-label={table.title}>
			<thead><tr>{table.columns.map((column, index) => <th key={index} scope="col">{column}</th>)}</tr></thead>
			{table.groups.map((group, index) => <tbody key={index}>
				{group.title !== null && <tr className="group"><th colSpan={3} scope="rowgroup">{group.title}</th></tr>}
				{group.rows.map((row, index) => {
					const ending = row.endings.split(', ')[0];
					return <tr key={index} className={row.hit ? 'hit' : undefined}>
						<th scope="row">{row.names.map((name, index) => <span key={index} className="kindrow">{row.hit && index === 0 ? <b className="right">{name}</b> : name}</span>)}</th>
						<td>{row.hit ? <><b className="right">{ending}</b>{row.endings.slice(ending.length)}</> : row.endings}</td>
						<td>{row.examples.map((text, index) => <span key={index} className="kindrow">{row.hit && index === 0 ? example(text) : text}</span>)}</td>
					</tr>;
				})}
			</tbody>)}
		</table>
	</>;
}
