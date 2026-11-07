import { useEffect, useRef, useState } from 'react';
import type { Competition, CompetitionAttempt, Grade } from '../shared/types';
import { api, ApiError } from './api';

const gradeNames: Record<Grade, string> = { correct: 'Dobrze', almost: 'Prawie dobrze', again: 'Powtórz' };

export function RaceHistory({ race }: { race: Competition }): React.JSX.Element {
	const [attempts, setAttempts] = useState<CompetitionAttempt[] | null>(null);
	const [error, setError] = useState('');
	const latest = useRef(0);

	async function load() {
		const request = ++latest.current;
		setError('');
		try {
			const result = await api<CompetitionAttempt[]>(`/competitions/${race.id}/attempts`);
			if (request === latest.current) setAttempts(result);
		} catch (error) {
			if (request === latest.current) setError(error instanceof ApiError ? error.message : 'Nie udało się wczytać prób. Sprawdź połączenie i spróbuj ponownie.');
		}
	}
	useEffect(() => { void load(); }, [race.id, race.version]);

	// Attempts come in race order, so a Map keeps the groups in the order of their first attempt.
	const groups = new Map<string, CompetitionAttempt[]>();
	for (const attempt of attempts ?? []) groups.set(attempt.cardId, [...groups.get(attempt.cardId) ?? [], attempt]);
	return <section className="race-history" aria-labelledby="race-history-title">
		<h2 id="race-history-title">Próby</h2>
		{error && <div className="notice"><p className="error" role="alert">{error}</p><button onClick={() => void load()}>Ponów</button></div>}
		{attempts?.length === 0 && <p className="hint">Nie ma jeszcze prób.</p>}
		{[...groups.values()].map(group => <div key={group[0].cardId}>
			<h3>{group[0].pl} / {group[0].ru}</h3>
			{group.map(attempt => {
				const lang = attempt.direction === 'ru-pl' ? 'pl' : 'ru';
				const name = race.participants.find(person => person.id === attempt.userId)?.displayName;
				return <details key={attempt.attemptId}>
					<summary>{name}: {attempt.answer === null ? 'Pokazano odpowiedź' : gradeNames[attempt.grade]}</summary>
					{attempt.answer !== null && <p>Odpowiedź: <span lang={lang}>{attempt.answer}</span></p>}
					<p>Poprawna odpowiedź: <span lang={lang}>{attempt.correction}</span></p>
					<p>{attempt.explanation}</p>
				</details>;
			})}
		</div>)}
	</section>;
}
