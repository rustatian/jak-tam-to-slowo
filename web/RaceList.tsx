import type { ReactNode } from 'react';
import { COMPETITION_TARGET, type Competition } from '../shared/types';
import type { useRaceList } from './useRaceList';
import { Icon } from './Icon';
import { directionShort, statusNames } from './race-text';

function details(race: Competition): string {
	const [a, b] = race.participants;
	const winner = race.participants.find(person => person.id === race.winnerId);
	return [
		new Date(race.createdAt).toLocaleDateString('pl-PL'), race.direction && directionShort[race.direction],
		`${a.displayName} ${a.score} : ${b.score} ${b.displayName}`, winner && `Wygrywa ${winner.displayName}`,
	].filter(Boolean).join(' · ');
}

export function RaceList({ list, locked, inviteDisabled, onNew, onOpen, onRestart, children }: {
	list: ReturnType<typeof useRaceList>; locked: boolean; inviteDisabled: boolean;
	onNew(): void; onOpen(id: string): void; onRestart(race: Competition): void; children: ReactNode;
}): React.JSX.Element {
	return <section className="page competition" aria-labelledby="competition-title">
		<h1 id="competition-title">Zawody</h1>
		<div className="competition-actions"><button className="primary" disabled={inviteDisabled} onClick={onNew}>Nowy wyścig</button></div>
		{children}
		{!list.races.length && !list.error && <p role="status">{list.loading ? 'Wczytywanie wyścigów…' : 'Nie ma jeszcze wyścigów.'}</p>}
		<ul className="race-list">{list.races.map(race => <li key={race.id} data-testid="race-row" data-race-id={race.id}>
			<button className="race-open" disabled={locked} onClick={() => onOpen(race.id)}>
				{/* The list has no cancelled races. */}
				<span>Wyścig do {race.target} <span className="race-chip" data-status={race.status}>{statusNames[race.status as keyof typeof statusNames]}</span></span>
				<span className="hint">{details(race)}</span>
			</button>
			{race.status === 'finished' && <button className="race-restart" aria-label="Zagraj jeszcze raz" disabled={inviteDisabled} onClick={() => onRestart(race)}><Icon name="again" /></button>}
		</li>)}</ul>
		{list.error && <div className="notice"><p className="error" role="alert">{list.error}</p><button disabled={list.loading} onClick={list.reload}>Ponów</button></div>}
		{list.nextCursor && <button disabled={locked || list.loading} onClick={list.more}>Więcej wyścigów</button>}
		<details className="competition-rules"><summary>Zasady wyścigu</summary>
			<p>Wyścig zaczyna się po przyjęciu zaproszenia. Wygrywa osoba, która pierwsza zdobędzie {COMPETITION_TARGET} punktów.</p>
			<p>Wynik zmienia się tylko po odpowiedziach w treningu wyścigu („Trenuj słówka”). Odpowiedzi w Nauce nie zmieniają wyniku. „Prawie dobrze”, „Powtórz” i „Pokaż odpowiedź” odejmują punkt. Przy zerze wynik pozostaje bez zmian.</p>
			<p>Liczą się tylko odpowiedzi w kierunku wyścigu. Po zakończeniu każda osoba może zacząć nowy wyścig przyciskiem „Zagraj jeszcze raz”.</p>
		</details>
	</section>;
}
