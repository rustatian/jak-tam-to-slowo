import { useEffect, useRef, useState } from 'react';
import type { Competition, CompetitionPage } from '../shared/types';
import { api, ApiError } from './api';

export function useRaceList(round: Competition | null) {
	const [page, setPage] = useState<CompetitionPage & { pages: number }>({ competitions: [], nextCursor: null, pages: 0 });
	const [loading, setLoading] = useState(true);
	const [error, setError] = useState('');
	const latest = useRef(0);

	async function load(cursor?: string) {
		const request = ++latest.current;
		setLoading(true);
		setError('');
		try {
			const result = await api<CompetitionPage>(`/competitions${cursor ? `?${new URLSearchParams({ cursor })}` : ''}`);
			if (request === latest.current) setPage(current => {
				if (cursor) return { competitions: [...current.competitions, ...result.competitions], nextCursor: result.nextCursor, pages: current.pages + 1 };
				const last = result.competitions.at(-1);
				// One loaded page is replaced: its older rows would come again with the next page.
				if (current.pages < 2 || !last) return { ...result, pages: 1 };
				// With more pages loaded, the rows after the new first page stay, so a race opened from a later page stays on screen.
				const older = current.competitions.filter(race => race.createdAt < last.createdAt || race.createdAt === last.createdAt && race.id < last.id);
				return { competitions: [...result.competitions, ...older], nextCursor: current.nextCursor, pages: current.pages };
			});
		} catch (error) {
			if (request === latest.current) setError(error instanceof ApiError ? error.message : 'Nie udało się wczytać wyścigów. Sprawdź połączenie i spróbuj ponownie.');
		} finally {
			if (request === latest.current) setLoading(false);
		}
	}
	useEffect(() => { void load(); }, [round?.id, round?.status, round?.version]);

	// The polled round is newer than its list row. A new invitation is not in the list yet.
	const races = !round ? page.competitions : page.competitions.some(race => race.id === round.id)
		? page.competitions.map(race => race.id === round.id ? round : race) : [round, ...page.competitions];
	return {
		races, nextCursor: page.nextCursor, loading, error,
		more: () => { if (page.nextCursor) void load(page.nextCursor); },
		reload: () => { void load(); },
	};
}
