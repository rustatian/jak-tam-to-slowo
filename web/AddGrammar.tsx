import { useEffect, useRef, useState, type FormEvent } from 'react';
import type { Card, CardPage, DraftResult, GrammarContent } from '../shared/types';
import { api, ApiError } from './api';
import { Dictation, dictatedValue } from './Dictation';
import { FilledSentence, FormPair, GrammarDetails } from './Grammar';

interface Draft {
	id: string;
	word: string;
	used: string;
	card?: GrammarContent;
	submission?: string;
}

export function AddGrammar({ userId, onOpen, busy, onBusy: setBusy }: { userId: string; onOpen(card: Card): void; busy: boolean; onBusy(busy: boolean): void }) {
	const key = `grammar-draft:${userId}`;
	const [draft, setDraft] = useState<Draft | null>(() => {
		try { return JSON.parse(sessionStorage.getItem(key) ?? 'null'); } catch { return null; }
	});
	const [error, setError] = useState('');
	const [storageError, setStorageError] = useState('');
	const [saved, setSaved] = useState<Card | null>(null);
	// null until the search for the current word ends.
	const [matches, setMatches] = useState<Card[] | null>(null);
	const [searchError, setSearchError] = useState('');
	const [dictationId, setDictationId] = useState(0);
	const wordInput = useRef<HTMLInputElement>(null), usedInput = useRef<HTMLInputElement>(null);
	// Set when a new card arrives, so the page scrolls to it once.
	const reveal = useRef(false);
	const query = draft?.word.trim() ?? '';
	const locked = busy || !!draft?.submission;

	useEffect(() => {
		try {
			if (draft) sessionStorage.setItem(key, JSON.stringify(draft));
			else sessionStorage.removeItem(key);
			setStorageError('');
		} catch { setStorageError('Przeglądarka nie zapisała szkicu. Skopiuj tekst przed zamknięciem strony.'); }
	}, [draft, key]);

	useEffect(() => {
		let cancelled = false;
		setSearchError('');
		// The previous matches stay until the next search ends, so the list does not blink on each key.
		if (!query) { setMatches(null); return; }
		const timer = setTimeout(() => {
			void api<CardPage>(`/cards?${new URLSearchParams({ q: query })}`).then(page => {
				if (!cancelled) setMatches(page.cards);
			}).catch(() => {
				if (cancelled) return;
				setMatches(null);
				setSearchError('Nie udało się wyszukać podobnych słówek. Sprawdź kolekcję w sekcji „Słówka”.');
			});
		}, 250);
		return () => { cancelled = true; clearTimeout(timer); };
	}, [query]);

	useEffect(() => {
		if (!reveal.current) return;
		reveal.current = false;
		document.getElementById('draft-result')?.scrollIntoView({ block: 'start' });
	}, [draft?.card]);

	function change(value: Partial<Draft>) {
		setDraft(current => ({ ...(current ?? { id: crypto.randomUUID(), word: '', used: '' }), ...value }));
		setError('');
		setSaved(null);
	}

	async function generate() {
		if (!draft || draft.submission) return;
		setBusy(true);
		setError('');
		setSaved(null);
		try {
			const result = await api<Extract<DraftResult, { kind: 'grammar' }>>('/cards/draft', { method: 'POST', body: JSON.stringify({ kind: 'grammar', word: draft.word, used: draft.used }) });
			reveal.current = true;
			change({ card: result.card });
		} catch (error) {
			setError(error instanceof ApiError ? error.message : 'Brak połączenia z serwerem. Szkic został w formularzu. Spróbuj ponownie.');
		}
		finally { setBusy(false); }
	}

	async function save(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		if (!draft?.card) return;
		const pending = { ...draft, submission: draft.submission ?? JSON.stringify({ id: draft.id, kind: 'grammar', ...draft.card }) };
		try {
			sessionStorage.setItem(key, JSON.stringify(pending));
			setStorageError('');
		} catch { setStorageError('Przeglądarka nie zapisała szkicu. Zwolnij miejsce i zapisz ponownie.'); return; }
		setDraft(pending);
		setBusy(true);
		setError('');
		try {
			const card = await api<Card>('/cards', { method: 'POST', body: pending.submission });
			setDraft(null);
			setSaved(card);
		} catch (error) {
			if (error instanceof ApiError && error.code === 'card_conflict') setDraft({ ...pending, id: crypto.randomUUID(), submission: undefined });
			else if (error instanceof ApiError && (error.status === 400 || error.status === 413)) setDraft({ ...pending, submission: undefined });
			setError(error instanceof ApiError ? error.message : 'Brak połączenia z serwerem. Zapis nie został potwierdzony. Spróbuj zapisać ponownie.');
		}
		finally { setBusy(false); }
	}

	const card = draft?.card;
	return <>
		<form onSubmit={event => { event.preventDefault(); void generate(); }}>
			<fieldset disabled={locked}>
				<label htmlFor="grammar-word">Słowo po polsku</label>
				<div className="dictation">
					<input ref={wordInput} id="grammar-word" lang="pl" value={draft?.word ?? ''} maxLength={200} required onChange={event => change({ word: event.target.value, card: undefined })} />
					{!card && <Dictation key={dictationId} language="pl" disabled={locked} onText={text => { if (wordInput.current) change({ word: dictatedValue(wordInput.current, text), card: undefined }); }} />}
				</div>
				<label htmlFor="grammar-used">Jak tego użyłeś</label>
				<div className="dictation">
					<input ref={usedInput} id="grammar-used" lang="pl" aria-describedby="grammar-used-hint" value={draft?.used ?? ''} maxLength={200} required onChange={event => change({ used: event.target.value, card: undefined })} />
					{!card && <Dictation key={dictationId} language="pl" disabled={locked} onText={text => { if (usedInput.current) change({ used: dictatedValue(usedInput.current, text), card: undefined }); }} />}
				</div>
				<p className="hint" id="grammar-used-hint">Słowo albo krótkie wyrażenie z błędem. Bez tłumaczenia i bez całego zdania.</p>
			</fieldset>
			<aside className="similar" aria-label="Podobne słówka">{searchError ? <p className="hint" role="status">{searchError}</p>
				: !query ? <p className="hint">Tu pojawią się podobne słówka z kolekcji.</p>
				: !matches ? <p className="hint">Szukanie podobnych słówek…</p>
				: !matches.length ? <p className="hint">Brak podobnych słówek.</p>
				: <><span className="hint">Już w kolekcji:</span><ul>{matches.map(card => <li key={card.id}>
					<a href="#words" aria-disabled={busy} onClick={event => { event.preventDefault(); if (!busy) onOpen(card); }}><span lang="pl">{card.pl}</span> / <span lang="ru">{card.ru}</span></a>
				</li>)}</ul></>}</aside>
			<button className="primary" disabled={locked || !query || !draft?.used.trim()}>Sprawdź gramatykę</button>
		</form>
		{draft?.submission && <p className="hint">Zapis nie został potwierdzony. Zapisz ponownie. Po potwierdzeniu fiszkę można zmienić w sekcji „Słówka”.</p>}
		{card && <form id="draft-result" onSubmit={save}>
			<h2>Sprawdź fiszkę</h2>
			<p className="hint">Po zapisaniu fiszka pojawi się u obu osób.</p>
			<div className="result">
				<FormPair grammar={card.grammar} />
				<p className="sentence"><FilledSentence sentence={card.examplePl} answer={card.grammar.answer} /></p>
				<GrammarDetails card={card} />
			</div>
			<fieldset disabled={locked}>
				<label htmlFor="grammar-answer">Poprawna forma</label>
				<input id="grammar-answer" lang="pl" maxLength={200} required value={card.grammar.answer} onChange={event => change({ card: { ...card, alternativesPl: [], grammar: { ...card.grammar, answer: event.target.value } } })} />
				<label htmlFor="grammar-example">Zdanie z luką</label>
				<textarea id="grammar-example" lang="pl" maxLength={500} required rows={2} value={card.examplePl} onChange={event => change({ card: { ...card, examplePl: event.target.value } })} />
				{card.alternativesPl.length > 0 && <p className="hint">Inne poprawne odpowiedzi: {card.alternativesPl.join(', ')}</p>}
				<p className="hint">Etykiety i tabelę przygotowuje AI. Jeśli coś się nie zgadza, sprawdź gramatykę jeszcze raz.</p>
			</fieldset>
			<button className="primary" disabled={busy} type="submit">Zapisz</button>
		</form>}
		{error && <p className="error" role="alert">{error}</p>}
		{storageError && <p className="error" role="alert">{storageError}</p>}
		{busy && <p role="status">Proszę czekać…</p>}
		{saved && <><p role="status">Fiszka zapisana.</p><a className="word" href="#words" onClick={event => { event.preventDefault(); onOpen(saved); }}>Otwórz w „Słówkach”</a></>}
		{draft && <button className="discard" disabled={locked} onClick={() => { setDraft(null); setDictationId(value => value + 1); setError(''); setSaved(null); }}>Usuń szkic</button>}
	</>;
}
