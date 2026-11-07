import { useState, type ReactNode } from 'react';
import { COMPETITION_TARGET, type Direction, type User } from '../shared/types';

export function NewRace({ user, locked, onBack, onInvite, children }: {
	user: User; locked: boolean; onBack(): void; onInvite(direction: Direction): void; children: ReactNode;
}): React.JSX.Element {
	const [direction, setDirection] = useState(user.direction);
	return <section className="page competition" aria-labelledby="competition-title">
		<button className="quiet back" aria-label="Wróć do listy wyścigów" disabled={locked} onClick={onBack}>‹ Wróć</button>
		<h1 id="competition-title">Nowy wyścig</h1>
		<p className="intro">Wyścig do {COMPETITION_TARGET}. Dobra odpowiedź: +1. Błąd lub pokazanie odpowiedzi: -1.</p>
		<p className="field-label" id="new-race-direction">Kierunek tłumaczenia</p>
		<div className="directions" role="group" aria-labelledby="new-race-direction">
			<button aria-label="RU -> PL" aria-pressed={direction === 'ru-pl'} disabled={locked} onClick={() => setDirection('ru-pl')}>Rosyjski <span aria-hidden="true">→</span> Polski</button>
			<button aria-label="PL -> RU" aria-pressed={direction === 'pl-ru'} disabled={locked} onClick={() => setDirection('pl-ru')}>Polski <span aria-hidden="true">→</span> Rosyjski</button>
		</div>
		<p className="hint">Obie osoby tłumaczą w tym kierunku do końca wyścigu.</p>
		<div className="competition-actions"><button className="primary" disabled={locked} onClick={() => onInvite(direction)}>Zaproś do wyścigu</button></div>
		{children}
	</section>;
}
