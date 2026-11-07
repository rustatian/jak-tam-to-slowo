import type { ReactNode } from 'react';
import type { Competition, User } from '../shared/types';
import type { CompetitionState } from './useCompetition';
import { competitionStatus } from './CompetitionSummary';
import { CompetitionLadder } from './CompetitionLadder';
import { RaceHistory } from './RaceHistory';
import { Icon } from './Icon';
import { directionNames } from './race-text';

export function RaceView({ user, race, competition, locked, restartDisabled, onBack, onTrain, onRestart, children }: {
	user: User; race: Competition; competition: CompetitionState; locked: boolean; restartDisabled: boolean;
	onBack(): void; onTrain(): void; onRestart(): void; children: ReactNode;
}): React.JSX.Element {
	// Accept and cancel act on the current competition only.
	const current = race.id === competition.round?.id;
	return <section className="page competition" aria-labelledby="competition-title">
		<button className="quiet back" aria-label="Wróć do listy wyścigów" disabled={locked} onClick={onBack}>‹ Wróć</button>
		<h1 id="competition-title">Wyścig do {race.target}</h1>
		{race.direction && <p className="intro">{directionNames[race.direction]}</p>}
		<div className="competition-scores" data-testid="competition-scores" role="status" aria-atomic="true">
			<div className="competition-score-row">{race.participants.map(person => <p key={person.id}><span>{person.displayName}{person.id === user.id && ' (Ty)'}: </span><strong>{person.score} <small>/ {race.target}</small></strong></p>)}</div>
			<CompetitionLadder round={race} />
			<p className="competition-lead">{competitionStatus(race)}</p>
		</div>
		<div className="competition-actions">
			{current && competition.actions.includes('accept') && <button className="primary" disabled={locked} onClick={competition.accept}>Przyjmij wyzwanie</button>}
			{current && competition.actions.includes('cancel') && <button disabled={locked} onClick={competition.cancel}>Anuluj zaproszenie</button>}
			{race.status === 'active' && race.direction && <button className="primary" disabled={locked} onClick={onTrain}>Trenuj słówka</button>}
			{race.status === 'finished' && <button className="primary restart" disabled={restartDisabled} onClick={onRestart}><Icon name="again" />Zagraj jeszcze raz</button>}
		</div>
		{children}
		<RaceHistory key={race.id} race={race} />
	</section>;
}
