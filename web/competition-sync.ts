import type { Competition } from '../shared/types';
import { api, ApiError } from './api';

export type LifecycleAction = 'invite' | 'accept' | 'cancel';

export interface CompetitionSnapshot {
	round: Competition | null;
	loading: boolean;
	readError: string;
	actionError: string;
	retryAction: LifecycleAction | null;
	busy: boolean;
	// The lifecycle actions the learner can start now.
	actions: LifecycleAction[];
}

export interface CompetitionSync {
	snapshot(): CompetitionSnapshot;
	subscribe(listener: () => void): () => void;
	refresh(): Promise<void>;
	poll(): Promise<void>;
	invite(): Promise<void>;
	accept(): Promise<void>;
	cancel(): Promise<void>;
	retry(): Promise<void>;
	dispose(): void;
}

export const idle: CompetitionSnapshot = { round: null, loading: false, readError: '', actionError: '', retryAction: null, busy: false, actions: [] };

// One instance serves one signed-in learner. dispose() drops every later response.
export function createCompetitionSync(userId: string): CompetitionSync {
	let active = true, generation = 0, queued = false, busy = false;
	let round: Competition | null = null;
	let pending: { action: LifecycleAction; id: string } | null = null;
	let flight: Promise<void> | null = null;
	let state = idle;
	const listeners = new Set<() => void>();

	function offered({ round, loading, readError, retryAction }: CompetitionSnapshot): LifecycleAction[] {
		if (retryAction) return [];
		if (!round) return loading || readError ? [] : ['invite'];
		if (round.status === 'waiting') return round.createdBy === userId ? ['cancel'] : ['accept', 'cancel'];
		return round.status === 'finished' ? ['invite'] : [];
	}
	function update(value: Partial<CompetitionSnapshot>) {
		if (!active) return;
		const next = { ...state, ...value };
		state = { ...next, actions: offered(next) };
		listeners.forEach(listener => listener());
	}
	function read(): Promise<void> {
		if (flight) return flight;
		flight = (async () => {
			do {
				queued = false;
				const version = generation;
				try {
					const saved = await api<Competition | null>('/competitions/current');
					if (!active || version !== generation) continue;
					if (!round || !saved || round.id !== saved.id || saved.version >= round.version) round = saved;
					update({ round, readError: '' });
					if (pending && (
						pending.action === 'invite' && saved && (saved.id === pending.id || saved.status === 'waiting' || saved.status === 'active') ||
						pending.action === 'accept' && saved?.id === pending.id && (saved.status === 'active' || saved.status === 'finished') ||
						pending.action === 'cancel' && saved?.id !== pending.id
					)) {
						pending = null;
						update({ actionError: '', retryAction: null });
					}
				} catch (error) {
					if (active && version === generation) update({ readError: error instanceof ApiError ? error.message : 'Nie udało się wczytać wyzwania. Sprawdź połączenie i spróbuj ponownie.' });
				}
			} while (active && queued);
			flight = null;
			update({ loading: false });
		})();
		return flight;
	}
	function refresh(): Promise<void> {
		// A write needs a read that starts after the write, even when a poll is still pending.
		generation++;
		queued = true;
		update({ loading: true });
		return read();
	}
	async function mutate(action: LifecycleAction, id = action === 'invite' ? crypto.randomUUID() : round?.id): Promise<void> {
		if (busy || !active || !id) return;
		busy = true;
		generation++;
		pending = null;
		update({ busy: true, actionError: '', retryAction: null });
		try {
			await api<Competition>(action === 'invite' ? '/competitions' : `/competitions/${id}/${action}`, {
				method: 'POST', body: JSON.stringify(action === 'invite' ? { id } : {}),
			});
		} catch (error) {
			if (!active) return;
			if (!(error instanceof ApiError) || error.status >= 500) pending = { action, id };
			update({ retryAction: pending?.action ?? null, actionError: error instanceof ApiError ? error.message : 'Nie udało się zapisać wyzwania. Sprawdź połączenie i ponów próbę.' });
		} finally {
			if (active) {
				await refresh();
				busy = false;
				update({ busy: false });
			}
		}
	}

	return {
		snapshot: () => state,
		subscribe(listener) {
			listeners.add(listener);
			return () => { listeners.delete(listener); };
		},
		refresh,
		// A timer read starts only when no read is in flight and no action is in progress.
		poll: () => flight || busy ? Promise.resolve() : read(),
		invite: () => mutate('invite'),
		accept: () => mutate('accept'),
		cancel: () => mutate('cancel'),
		retry: async () => { if (pending) await mutate(pending.action, pending.id); },
		dispose() { active = false; },
	};
}
