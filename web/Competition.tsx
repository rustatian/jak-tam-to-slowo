import { useState } from 'react';
import { COMPETITION_TARGET, type Direction, type ReviewResult, type User } from '../shared/types';
import type { CompetitionState } from './useCompetition';
import { competitionStatus } from './CompetitionSummary';
import { CompetitionLadder } from './CompetitionLadder';
import { RaceTraining } from './RaceTraining';

export function Competition({ user, competition, locked, onReview, onDirection, onBusy }: {
	user: User; competition: CompetitionState; locked: boolean; onReview(result: ReviewResult): void; onDirection(direction: Direction): void; onBusy(busy: boolean): void;
}): React.JSX.Element {
	const { round, loading, readError, actionError, retryAction, busy, actions } = competition;
	const [trained, setTrained] = useState<CompetitionState['round']>(null);
	// Race training keeps its race when the polled round moves to the next race.
	if (trained && round?.id === trained.id && round !== trained) setTrained(round);
	if (trained) return <RaceTraining user={user} round={trained} finished={trained.status === 'finished' || round?.id !== trained.id} stale={!!readError} busy={locked}
		onReview={onReview} onDirection={onDirection} onBusy={onBusy} onBack={() => setTrained(null)} />;
	return <section className="page competition" aria-labelledby="competition-title">
		<h1 id="competition-title">Wyścig do {round?.target ?? COMPETITION_TARGET}</h1>
		<p className="intro">Dobra odpowiedź: +1. Błąd lub pokazanie odpowiedzi: -1. Wynik nie spada poniżej zera.</p>
		{!round && loading && <p role="status">Wczytywanie wyzwania…</p>}
		<div className="competition-scores" data-testid={round ? 'competition-scores' : undefined} role="status" aria-atomic="true">
			{round && <div className="competition-score-row">{round.participants.map(person => <p key={person.id}><span>{person.displayName}{person.id === user.id && ' (Ty)'}: </span><strong>{person.score} <small>/ {round.target}</small></strong></p>)}</div>}
			<CompetitionLadder round={round} />
			{round && <p className="competition-lead">{competitionStatus(round)}</p>}
		</div>
		<div className="competition-actions">
			{actions.includes('invite') && <button className="primary" disabled={busy} onClick={() => void competition.invite(user.direction)}>{round ? 'Zaproś na rewanż' : 'Zaproś do wyścigu'}</button>}
			{actions.includes('accept') && <button className="primary" disabled={busy} onClick={competition.accept}>Przyjmij wyzwanie</button>}
			{actions.includes('cancel') && <button disabled={busy} onClick={competition.cancel}>Anuluj zaproszenie</button>}
			{round?.status === 'active' && round.direction && <button className="primary" disabled={busy} onClick={() => setTrained(round)}>Trenuj słówka</button>}
		</div>
		{busy && <p className="delayed" role="status">Zapisywanie wyzwania…</p>}
		{actionError && <div className="notice"><p className="error" role="alert">{actionError}</p>
			{retryAction && <button disabled={busy} onClick={competition.retry}>{retryAction === 'invite' ? 'Ponów zaproszenie' : retryAction === 'accept' ? 'Ponów przyjęcie' : 'Ponów anulowanie'}</button>}
		</div>}
		{readError && <div className="notice">
			{round && <p>Wynik może być nieaktualny</p>}
			<p className="error" role="alert">{readError}</p>
			<button disabled={busy || loading} onClick={competition.refresh}>{round ? 'Odśwież' : 'Ponów'}</button>
		</div>}
		<details className="competition-rules"><summary>Zasady wyścigu</summary>
			<p>Wyścig zaczyna się po przyjęciu zaproszenia. Wygrywa osoba, która pierwsza zdobędzie {round?.target ?? COMPETITION_TARGET} punktów.</p>
			<p>Wynik zmienia się tylko po odpowiedziach w treningu wyścigu („Trenuj słówka”). Odpowiedzi w Nauce nie zmieniają wyniku. „Prawie dobrze”, „Powtórz” i „Pokaż odpowiedź” odejmują punkt. Przy zerze wynik pozostaje bez zmian.</p>
			<p>Liczą się tylko odpowiedzi w kierunku wyścigu. Po zakończeniu możecie rozpocząć rewanż od zera.</p>
		</details>
	</section>;
}
