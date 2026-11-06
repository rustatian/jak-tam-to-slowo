import { env } from 'cloudflare:workers';
import { beforeEach, expect, it, vi } from 'vitest';
import worker from '../src/index';
import { setApiAccount } from '../web/api';
import { createCompetitionSync, type LifecycleAction } from '../web/competition-sync';
import { login, request } from './client';
import { USER_A, USER_B } from './fixtures';

const origin = 'https://learn.rustatian.me';
const current = '/api/competitions/current';
const roundId = '33333333-3333-4333-8333-333333333333';
const previousId = '55555555-5555-4555-8555-555555555555';
const nextId = '66666666-6666-4666-8666-666666666666';
const saveError = 'Nie udało się zapisać wyzwania. Sprawdź połączenie i ponów próbę.';

type Respond = (forward: () => Promise<Response>) => Promise<Response>;
let cookie = '';
let calls: { method: string; path: string; body: string | null }[] = [];
let overrides: { method: string; path: string; respond: Respond }[] = [];

// web/api.ts sends relative /api paths. The stub sends them to the Worker as the signed-in learner, unless a test replaces the next matching request.
beforeEach(() => {
	calls = [];
	overrides = [];
	vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
		const method = init?.method ?? 'GET', path = String(input), body = typeof init?.body === 'string' ? init.body : null;
		calls.push({ method, path, body });
		const forward = () => {
			const headers = new Headers(init?.headers);
			headers.set('Cookie', cookie);
			headers.set('Origin', origin);
			return worker.fetch(new Request(`${origin}${path}`, { method, headers, body }), env);
		};
		const index = overrides.findIndex(item => item.method === method && item.path === path);
		return index < 0 ? forward() : overrides.splice(index, 1)[0].respond(forward);
	});
});

async function signIn(): Promise<[string, string]> {
	const cookieA = await login(USER_A.id), cookieB = await login(USER_B.id);
	cookie = cookieA;
	setApiAccount(USER_A.id);
	return [cookieA, cookieB];
}

const invite = (from: string, id: string) => request('/api/competitions', { method: 'POST', cookie: from, body: { id } });
const act = (from: string, id: string, action: 'accept' | 'cancel') => request(`/api/competitions/${id}/${action}`, { method: 'POST', cookie: from });
const finish = (id: string) => env.DB.prepare(`UPDATE competitions SET score_a = 20, status = 'finished', winner_id = ?, finished_at = ?, version = version + 1 WHERE id = ?`)
	.bind(USER_A.id, Date.now(), id).run();
const posts = () => calls.filter(call => call.method === 'POST');
const reads = () => calls.filter(call => call.method === 'GET' && call.path === current).length;

function offline(): never {
	throw new TypeError('Network connection lost.');
}
// The server applies the request, and the response does not arrive.
const lost: Respond = async forward => {
	await forward();
	return offline();
};
const unavailable = (error: string): Respond => async () => Response.json({ error }, { status: 503 });
// The server answers at once, and the answer arrives only after release().
function held() {
	const served = Promise.withResolvers<void>(), gate = Promise.withResolvers<void>();
	const respond: Respond = async forward => {
		const response = await forward();
		served.resolve();
		await gate.promise;
		return response;
	};
	return { served: served.promise, respond, release: () => gate.resolve() };
}

it('reads once more after a held poll when the score changes', async () => {
	const [cookieA, cookieB] = await signIn();
	await invite(cookieA, roundId);
	await act(cookieB, roundId, 'accept');
	const sync = createCompetitionSync(USER_A.id);
	await sync.refresh();
	const poll = held();
	overrides.push({ method: 'GET', path: current, respond: poll.respond });
	const polled = sync.poll();
	await poll.served;
	await env.DB.prepare('UPDATE competitions SET score_a = 1, version = version + 1 WHERE id = ?').bind(roundId).run();
	const refreshed = sync.refresh();
	expect(reads()).toBe(2);
	poll.release();
	await Promise.all([polled, refreshed]);
	expect(reads()).toBe(3);
	expect(sync.snapshot().round?.participants.map(person => person.score)).toEqual([1, 0]);
});

it('offers no invitation while the first read is pending', async () => {
	await signIn();
	const sync = createCompetitionSync(USER_A.id);
	const read = held();
	overrides.push({ method: 'GET', path: current, respond: read.respond });
	const refreshed = sync.refresh();
	await read.served;
	expect(sync.snapshot()).toMatchObject({ round: null, loading: true, actions: [] });
	read.release();
	await refreshed;
	expect(sync.snapshot()).toMatchObject({ round: null, loading: false, actions: ['invite'] });
});

it('retries an unconfirmed invitation with its first ID', async () => {
	await signIn();
	const sync = createCompetitionSync(USER_A.id);
	await sync.refresh();
	overrides.push({ method: 'POST', path: '/api/competitions', respond: lost }, { method: 'GET', path: current, respond: unavailable('Brak wyniku.') });
	await sync.invite();
	expect(sync.snapshot()).toMatchObject({ round: null, actionError: saveError, readError: 'Brak wyniku.', retryAction: 'invite', busy: false, actions: [] });
	await sync.retry();
	const [first, retry] = posts();
	expect(retry.body).toBe(first.body);
	expect(await env.DB.prepare('SELECT count(*) FROM competitions').first('count(*)')).toBe(1);
	expect(sync.snapshot()).toMatchObject({
		round: { id: JSON.parse(first.body!).id, status: 'waiting' }, actionError: '', readError: '', retryAction: null, actions: ['cancel'],
	});
});

it('confirms an unconfirmed invitation from the current round and invites a rematch with a new ID', async () => {
	const [, cookieB] = await signIn();
	const sync = createCompetitionSync(USER_A.id);
	await sync.refresh();
	overrides.push({ method: 'POST', path: '/api/competitions', respond: lost });
	await sync.invite();
	const lostId = JSON.parse(posts()[0].body!).id;
	expect(sync.snapshot()).toMatchObject({ round: { id: lostId, status: 'waiting' }, actionError: '', retryAction: null, actions: ['cancel'] });
	await act(cookieB, lostId, 'accept');
	await finish(lostId);
	await sync.refresh();
	expect(sync.snapshot()).toMatchObject({ round: { id: lostId, status: 'finished' }, actions: ['invite'] });
	await sync.invite();
	const rematchId = JSON.parse(posts()[1].body!).id;
	expect(rematchId).not.toBe(lostId);
	expect(sync.snapshot()).toMatchObject({ round: { id: rematchId, status: 'waiting' }, actions: ['cancel'] });
});

it.each([
	{ name: 'acceptance from the read after it', previous: false, action: 'accept', poll: false, finish: false, expectedId: roundId, status: 'active', actions: [] },
	{ name: 'acceptance from a later poll of the finished round', previous: false, action: 'accept', poll: true, finish: true, expectedId: roundId, status: 'finished', actions: ['invite'] },
	{ name: 'first cancellation from the read after it', previous: false, action: 'cancel', poll: false, finish: false, expectedId: null, status: null, actions: ['invite'] },
	{ name: 'rematch cancellation from the read after it', previous: true, action: 'cancel', poll: false, finish: false, expectedId: previousId, status: 'finished', actions: ['invite'] },
	{ name: 'rematch cancellation from a later poll', previous: true, action: 'cancel', poll: true, finish: false, expectedId: previousId, status: 'finished', actions: ['invite'] },
] satisfies { name: string; previous: boolean; action: 'accept' | 'cancel'; poll: boolean; finish: boolean; expectedId: string | null; status: string | null; actions: LifecycleAction[] }[])(
	'confirms an unconfirmed $name', async ({ previous, action, poll, finish: finishRound, expectedId, status, actions }) => {
		const [cookieA, cookieB] = await signIn();
		if (previous) {
			await invite(cookieA, previousId);
			await act(cookieB, previousId, 'accept');
			await finish(previousId);
		}
		await invite(cookieB, roundId);
		const sync = createCompetitionSync(USER_A.id);
		await sync.refresh();
		overrides.push({ method: 'POST', path: `/api/competitions/${roundId}/${action}`, respond: lost });
		if (poll) overrides.push({ method: 'GET', path: current, respond: unavailable('Brak wyniku.') });
		await sync[action]();
		if (poll) {
			expect(sync.snapshot()).toMatchObject({ round: { id: roundId, status: 'waiting' }, actionError: saveError, readError: 'Brak wyniku.', retryAction: action });
			if (finishRound) await finish(roundId);
			await sync.poll();
		}
		expect(sync.snapshot()).toMatchObject({ actionError: '', readError: '', retryAction: null, busy: false, actions });
		expect(sync.snapshot().round?.id ?? null).toBe(expectedId);
		expect(sync.snapshot().round?.status ?? null).toBe(status);
	});

it('retries an unconfirmed acceptance against its own round after another invitation becomes current', async () => {
	const [cookieA, cookieB] = await signIn();
	await invite(cookieB, roundId);
	const sync = createCompetitionSync(USER_A.id);
	await sync.refresh();
	overrides.push({ method: 'POST', path: `/api/competitions/${roundId}/accept`, respond: lost }, { method: 'GET', path: current, respond: unavailable('Brak wyniku.') });
	await sync.accept();
	await finish(roundId);
	await invite(cookieA, nextId);
	await sync.refresh();
	expect(sync.snapshot()).toMatchObject({ round: { id: nextId, status: 'waiting' }, actionError: saveError, retryAction: 'accept', actions: [] });
	await sync.retry();
	expect(posts().map(call => call.path)).toEqual([`/api/competitions/${roundId}/accept`, `/api/competitions/${roundId}/accept`]);
	expect(sync.snapshot()).toMatchObject({ round: { id: nextId, status: 'waiting' }, actionError: '', retryAction: null, actions: ['cancel'] });
});

it('retries an unconfirmed cancellation against its own round', async () => {
	const [, cookieB] = await signIn();
	await invite(cookieB, roundId);
	const sync = createCompetitionSync(USER_A.id);
	await sync.refresh();
	overrides.push({ method: 'POST', path: `/api/competitions/${roundId}/cancel`, respond: lost }, { method: 'GET', path: current, respond: unavailable('Brak wyniku.') });
	await sync.cancel();
	expect(sync.snapshot()).toMatchObject({ round: { id: roundId, status: 'waiting' }, actionError: saveError, retryAction: 'cancel', actions: [] });
	await sync.retry();
	expect(posts().map(call => call.path)).toEqual([`/api/competitions/${roundId}/cancel`, `/api/competitions/${roundId}/cancel`]);
	expect(sync.snapshot()).toMatchObject({ round: null, actionError: '', readError: '', retryAction: null, actions: ['invite'] });
});

it('confirms an unconfirmed invitation when the other learner invites at the same time', async () => {
	const [, cookieB] = await signIn();
	const sync = createCompetitionSync(USER_A.id);
	await sync.refresh();
	overrides.push({ method: 'POST', path: '/api/competitions', respond: async () => {
		await invite(cookieB, roundId);
		return offline();
	} });
	await sync.invite();
	expect(sync.snapshot()).toMatchObject({ round: { id: roundId, status: 'waiting' }, actionError: '', retryAction: null, actions: ['accept', 'cancel'] });
});

it('drops an unconfirmed acceptance when its retry gets a conflict', async () => {
	const [, cookieB] = await signIn();
	await invite(cookieB, roundId);
	const sync = createCompetitionSync(USER_A.id);
	await sync.refresh();
	overrides.push({ method: 'POST', path: `/api/competitions/${roundId}/accept`, respond: async () => offline() });
	await sync.accept();
	expect(sync.snapshot()).toMatchObject({ round: { id: roundId, status: 'waiting' }, actionError: saveError, retryAction: 'accept', actions: [] });
	await act(cookieB, roundId, 'cancel');
	await sync.retry();
	expect(sync.snapshot()).toMatchObject({ round: null, actionError: 'Stan wyzwania już się zmienił. Odśwież wyzwanie.', retryAction: null, actions: ['invite'] });
	await sync.retry();
	expect(posts().map(call => call.path)).toEqual([`/api/competitions/${roundId}/accept`, `/api/competitions/${roundId}/accept`]);
});

it('keeps a conflict error without a retry and shows the current round', async () => {
	const [cookieA, cookieB] = await signIn();
	await invite(cookieA, roundId);
	const sync = createCompetitionSync(USER_A.id);
	await sync.refresh();
	overrides.push({ method: 'POST', path: `/api/competitions/${roundId}/cancel`, respond: async forward => {
		await act(cookieB, roundId, 'accept');
		return forward();
	} });
	await sync.cancel();
	expect(sync.snapshot()).toMatchObject({
		round: { id: roundId, status: 'active' }, actionError: 'Stan wyzwania już się zmienił. Odśwież wyzwanie.', retryAction: null, busy: false, actions: [],
	});
});

it('ignores a read that ends after dispose', async () => {
	const [, cookieB] = await signIn();
	const sync = createCompetitionSync(USER_A.id);
	await sync.refresh();
	await invite(cookieB, roundId);
	const before = sync.snapshot();
	const poll = held();
	overrides.push({ method: 'GET', path: current, respond: poll.respond });
	const polled = sync.poll();
	await poll.served;
	const listener = vi.fn();
	sync.subscribe(listener);
	sync.dispose();
	poll.release();
	await polled;
	expect(listener).not.toHaveBeenCalled();
	expect(sync.snapshot()).toBe(before);
});

it('stays busy until the read after an action ends', async () => {
	const [cookieA] = await signIn();
	await invite(cookieA, roundId);
	const sync = createCompetitionSync(USER_A.id);
	await sync.refresh();
	const read = held();
	overrides.push({ method: 'GET', path: current, respond: read.respond });
	const cancelled = sync.cancel();
	await read.served;
	expect(sync.snapshot()).toMatchObject({ round: { id: roundId, status: 'waiting' }, busy: true });
	read.release();
	await cancelled;
	expect(sync.snapshot()).toMatchObject({ round: null, busy: false, actions: ['invite'] });
});

it('keeps the last round after a failed read and clears the error on refresh', async () => {
	const [cookieA] = await signIn();
	await invite(cookieA, roundId);
	const sync = createCompetitionSync(USER_A.id);
	await sync.refresh();
	overrides.push({ method: 'GET', path: current, respond: unavailable('Brak wyniku.') });
	await sync.refresh();
	expect(sync.snapshot()).toMatchObject({ round: { id: roundId, status: 'waiting' }, readError: 'Brak wyniku.', actions: ['cancel'] });
	await sync.refresh();
	expect(sync.snapshot()).toMatchObject({ round: { id: roundId, status: 'waiting' }, readError: '' });
});

it('offers no action until a failed first read recovers', async () => {
	const [, cookieB] = await signIn();
	await invite(cookieB, roundId);
	overrides.push({ method: 'GET', path: current, respond: unavailable('Brak wyzwania.') });
	const sync = createCompetitionSync(USER_A.id);
	await sync.refresh();
	expect(sync.snapshot()).toMatchObject({ round: null, readError: 'Brak wyzwania.', actions: [] });
	await sync.refresh();
	expect(sync.snapshot()).toMatchObject({ round: { id: roundId, status: 'waiting' }, readError: '', actions: ['accept', 'cancel'] });
});

it.each([
	{ name: 'no round', inviter: null, status: null, actions: ['invite'] },
	{ name: 'an own invitation', inviter: 'learner', status: 'waiting', actions: ['cancel'] },
	{ name: 'an invitation from the other learner', inviter: 'other', status: 'waiting', actions: ['accept', 'cancel'] },
	{ name: 'an active round', inviter: 'other', status: 'active', actions: [] },
	{ name: 'a finished round', inviter: 'other', status: 'finished', actions: ['invite'] },
] satisfies { name: string; inviter: 'learner' | 'other' | null; status: 'waiting' | 'active' | 'finished' | null; actions: LifecycleAction[] }[])(
	'offers the lifecycle actions for $name', async ({ inviter, status, actions }) => {
		const [cookieA, cookieB] = await signIn();
		if (inviter) await invite(inviter === 'learner' ? cookieA : cookieB, roundId);
		if (status === 'active' || status === 'finished') await act(cookieA, roundId, 'accept');
		if (status === 'finished') await finish(roundId);
		const sync = createCompetitionSync(USER_A.id);
		await sync.refresh();
		expect(sync.snapshot().round?.status ?? null).toBe(status);
		expect(sync.snapshot().actions).toEqual(actions);
	});
