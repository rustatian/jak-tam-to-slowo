import { useEffect } from 'react';
import type { Competition as Race, Direction, ReviewResult, User } from '../shared/types';
import type { CompetitionState } from './useCompetition';
import { useRaceList } from './useRaceList';
import { RaceList } from './RaceList';
import { NewRace } from './NewRace';
import { RaceView } from './RaceView';
import { RaceTraining } from './RaceTraining';

export type ZawodyView = { kind: 'list' } | { kind: 'new' } | { kind: 'race'; id: string } | { kind: 'training'; id: string };

export function Competition({ user, competition, locked, view, onView, onReview, onDirection, onBusy }: {
	user: User; competition: CompetitionState; locked: boolean; view: ZawodyView; onView(view: ZawodyView): void;
	onReview(result: ReviewResult): void; onDirection(direction: Direction): void; onBusy(busy: boolean): void;
}): React.JSX.Element {
	const { round, loading, readError, actionError, retryAction, busy, actions } = competition;
	const list = useRaceList(round);
	const race = view.kind === 'race' || view.kind === 'training' ? list.races.find(item => item.id === view.id) : undefined;
	const missing = (view.kind === 'race' || view.kind === 'training') && !race;
	// A race leaves the list when the other learner cancels it.
	useEffect(() => { if (missing && !list.loading) onView({ kind: 'list' }); }, [missing, list.loading, onView]);
	const inviteDisabled = locked || !actions.includes('invite');

	async function show(action: Promise<Race | null>) {
		const next = await action;
		if (next) onView({ kind: 'race', id: next.id });
	}
	function restart(race: Race) {
		if (race.direction) void show(competition.invite(race.direction));
		else onView({ kind: 'new' });
	}

	if (view.kind === 'training' && race) return <RaceTraining user={user} round={race} finished={race.status === 'finished' || round?.id !== race.id} stale={!!readError} busy={locked}
		onReview={onReview} onDirection={onDirection} onBusy={onBusy} onBack={() => onView({ kind: 'race', id: race.id })} onRestart={() => restart(race)} restartDisabled={inviteDisabled} />;
	const status = <>
		{busy && <p className="delayed" role="status">Zapisywanie wyzwania…</p>}
		{actionError && <div className="notice"><p className="error" role="alert">{actionError}</p>
			{retryAction && <button disabled={busy} onClick={() => void (retryAction === 'invite' ? show(competition.retry()) : competition.retry())}>{retryAction === 'invite' ? 'Ponów zaproszenie' : retryAction === 'accept' ? 'Ponów przyjęcie' : 'Ponów anulowanie'}</button>}
		</div>}
		{readError && <div className="notice">
			{round && <p>Wynik może być nieaktualny</p>}
			<p className="error" role="alert">{readError}</p>
			<button disabled={busy || loading} onClick={competition.refresh}>{round ? 'Odśwież' : 'Ponów'}</button>
		</div>}
	</>;
	if (view.kind === 'new') return <NewRace user={user} locked={locked} onBack={() => onView({ kind: 'list' })} onInvite={direction => void show(competition.invite(direction))}>{status}</NewRace>;
	if (race) return <RaceView user={user} race={race} competition={competition} locked={locked} restartDisabled={inviteDisabled}
		onBack={() => onView({ kind: 'list' })} onTrain={() => onView({ kind: 'training', id: race.id })} onRestart={() => restart(race)}>{status}</RaceView>;
	return <RaceList list={list} locked={locked} inviteDisabled={inviteDisabled} onNew={() => onView({ kind: 'new' })} onOpen={id => onView({ kind: 'race', id })} onRestart={restart}>{status}</RaceList>;
}
