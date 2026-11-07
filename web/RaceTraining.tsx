import { useState } from 'react';
import type { Competition, Direction, ReviewResult, User } from '../shared/types';
import { CompetitionSummary, competitionStatus } from './CompetitionSummary';
import { Study } from './Study';

export function RaceTraining({ user, round, finished, stale, busy, onReview, onDirection, onBusy, onBack }: {
	user: User; round: Competition; finished: boolean; stale: boolean; busy: boolean;
	onReview(result: ReviewResult): void; onDirection(direction: Direction): void; onBusy(busy: boolean): void; onBack(): void;
}): React.JSX.Element {
	const [ended, setEnded] = useState(false);
	// The Zawody screen opens race training only for a race with a direction.
	const race = { id: round.id, direction: round.direction!, finished };
	return <>
		<CompetitionSummary userId={user.id} round={round} stale={stale} busy={busy} onOpen={onBack} />
		{ended ? <div className="race-result">
			<p className="competition-lead">{competitionStatus(round)}</p>
			<div className="competition-actions"><button aria-label="Wróć do wyścigu" disabled={busy} onClick={onBack}>Wróć</button></div>
		</div> : <Study user={user} race={race} onReview={onReview} onDirection={onDirection} onBusy={onBusy} onEnded={() => setEnded(true)} />}
	</>;
}
