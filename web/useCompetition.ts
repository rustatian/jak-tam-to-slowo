import { useEffect, useState, useSyncExternalStore } from 'react';
import type { User } from '../shared/types';
import { createCompetitionSync, idle, type CompetitionSnapshot, type CompetitionSync } from './competition-sync';

export type CompetitionState = CompetitionSnapshot & Pick<CompetitionSync, 'refresh' | 'invite' | 'accept' | 'cancel' | 'retry'>;

const starting: CompetitionSnapshot = { ...idle, loading: true };
const unsubscribed = () => () => {};
const nothing = async () => {};
const nobody = async () => null;

export function useCompetition(user: User | null): CompetitionState {
	const userId = user?.id ?? null;
	const [current, setCurrent] = useState<{ userId: string; sync: CompetitionSync } | null>(null);

	useEffect(() => {
		if (!userId) return;
		const sync = createCompetitionSync(userId);
		setCurrent({ userId, sync });
		function visible() { if (document.visibilityState === 'visible') void sync.refresh(); }
		const timer = setInterval(() => { if (document.visibilityState === 'visible') void sync.poll(); }, 5000);
		document.addEventListener('visibilitychange', visible);
		void sync.refresh();
		return () => {
			sync.dispose();
			setCurrent(null);
			clearInterval(timer);
			document.removeEventListener('visibilitychange', visible);
		};
	}, [userId]);

	// The first render after an account change runs before the effect creates the new instance.
	const sync = current?.userId === userId ? current.sync : null;
	const snapshot = useSyncExternalStore(sync?.subscribe ?? unsubscribed, sync?.snapshot ?? (() => userId ? starting : idle));
	return {
		...snapshot,
		refresh: sync?.refresh ?? nothing, invite: sync?.invite ?? nobody, accept: sync?.accept ?? nothing,
		cancel: sync?.cancel ?? nothing, retry: sync?.retry ?? nobody,
	};
}
