import { COMPETITION_TARGET, type Competition, type CompetitionAttempt, type CompetitionPage, type User } from '../shared/types';
import type { Env } from './index';
import { competitionAccounts } from './auth';
import { HttpError, methods, object, readJson, uuidPattern } from './http';

type CompetitionRow = Omit<Competition, 'participants'> & { idA: string; nameA: string; scoreA: number; idB: string; nameB: string; scoreB: number };

const select = `SELECT c.id, c.created_by AS createdBy, c.target, c.status, c.direction, c.winner_id AS winnerId,
	c.created_at AS createdAt, c.started_at AS startedAt, c.finished_at AS finishedAt, c.version,
	c.participant_a_id AS idA, a.display_name AS nameA, c.score_a AS scoreA,
	c.participant_b_id AS idB, b.display_name AS nameB, c.score_b AS scoreB
	FROM competitions c JOIN users a ON a.id = c.participant_a_id JOIN users b ON b.id = c.participant_b_id`;

function fromRow(row: CompetitionRow): Competition {
	const { idA, nameA, scoreA, idB, nameB, scoreB, ...round } = row;
	return { ...round, participants: [{ id: idA, displayName: nameA, score: scoreA }, { id: idB, displayName: nameB, score: scoreB }] };
}

const pageSize = 10;

async function listCompetitions(url: URL, env: Env, userId: string): Promise<CompetitionPage> {
	let time: number | null = null;
	let id: string | null = null;
	const cursor = url.searchParams.get('cursor');
	if (cursor !== null) {
		const parts = cursor.split(':');
		if (parts.length !== 2 || !/^\d{1,16}$/.test(parts[0]) || !Number.isSafeInteger(Number(parts[0])) || !uuidPattern.test(parts[1])) {
			throw new HttpError(400, 'invalid_input', 'Nieprawidłowy wskaźnik strony. Odśwież listę.');
		}
		time = Number(parts[0]);
		id = parts[1];
	}
	const { results } = await env.DB.prepare(`${select} WHERE ?1 IN (c.participant_a_id, c.participant_b_id) AND c.status <> 'cancelled'
		AND (?2 IS NULL OR (c.created_at, c.id) < (?2, ?3)) ORDER BY c.created_at DESC, c.id DESC LIMIT ${pageSize + 1}`)
		.bind(userId, time, id).all<CompetitionRow>();
	const competitions = results.slice(0, pageSize).map(fromRow);
	const last = competitions.at(-1);
	return { competitions, nextCursor: results.length > pageSize && last ? `${last.createdAt}:${last.id}` : null };
}

interface AttemptRow extends Omit<CompetitionAttempt, 'grade' | 'correction' | 'explanation'> { result: string }

async function listAttempts(env: Env, id: string, userId: string): Promise<CompetitionAttempt[]> {
	const member = await env.DB.prepare('SELECT 1 FROM competitions WHERE id = ? AND ? IN (participant_a_id, participant_b_id)').bind(id, userId).first();
	if (!member) throw new HttpError(404, 'not_found', 'Nie znaleziono wyzwania.');
	const { results } = await env.DB.prepare(`SELECT e.user_id AS userId, e.attempt_id AS attemptId, cards.id AS cardId, cards.kind, cards.pl, cards.ru,
		json_extract(r.input_json, '$.direction') AS direction, json_extract(r.input_json, '$.answer') AS answer,
		r.result_json AS result, e.delta, e.score_after AS scoreAfter, e.version
		FROM competition_events e JOIN reviews r ON r.user_id = e.user_id AND r.attempt_id = e.attempt_id
		JOIN cards ON cards.id = json_extract(r.input_json, '$.cardId') WHERE e.competition_id = ? ORDER BY e.version`).bind(id).all<AttemptRow>();
	return results.map(({ result, ...row }) => {
		const { grade, correction, explanation } = JSON.parse(result);
		return { ...row, grade, correction, explanation };
	});
}

export async function handleCompetitions(request: Request, env: Env, user: User): Promise<Response | null> {
	const url = new URL(request.url);
	const { pathname } = url;
	if (pathname === '/api/competitions/current') {
		methods(request, ['GET']);
		const [open, finished] = await env.DB.batch<CompetitionRow>([
			env.DB.prepare(`${select} WHERE ? IN (c.participant_a_id, c.participant_b_id) AND c.status IN ('waiting', 'active')`).bind(user.id),
			env.DB.prepare(`${select} WHERE ? IN (c.participant_a_id, c.participant_b_id) AND c.status = 'finished'
				ORDER BY c.finished_at DESC, c.id DESC LIMIT 1`).bind(user.id),
		]);
		const row = open.results[0] ?? finished.results[0];
		return Response.json(row ? fromRow(row) : null);
	}
	if (pathname === '/api/competitions') {
		methods(request, ['GET', 'POST']);
		if (request.method === 'GET') return Response.json(await listCompetitions(url, env, user.id));
		const { id, direction } = object(await readJson(request), ['id', 'direction']);
		if (typeof id !== 'string' || !uuidPattern.test(id)) throw new HttpError(400, 'invalid_input', 'Nieprawidłowy identyfikator wyzwania.');
		if (direction !== 'ru-pl' && direction !== 'pl-ru') throw new HttpError(400, 'invalid_input', 'Wybierz kierunek wyścigu.');
		const accounts = competitionAccounts(env).sort((a, b) => a.id.localeCompare(b.id));
		if (accounts.length !== 2) throw new HttpError(503, 'competition_unavailable', 'Wyzwanie wymaga dwóch skonfigurowanych kont.');
		const [a, b] = accounts;
		const registered = await env.DB.prepare('SELECT count(*) AS count FROM users WHERE id IN (?, ?)').bind(a.id, b.id).first<number>('count');
		if (registered !== 2) throw new HttpError(409, 'opponent_not_ready', 'Druga osoba musi najpierw zalogować się do aplikacji.');
		const [, saved] = await env.DB.batch<CompetitionRow>([
			env.DB.prepare(`INSERT INTO competitions (id, participant_a_id, participant_b_id, target, created_by, direction, created_at)
				VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT DO NOTHING`).bind(id, a.id, b.id, COMPETITION_TARGET, user.id, direction, Date.now()),
			env.DB.prepare(`${select} WHERE ?2 IN (c.participant_a_id, c.participant_b_id) AND (c.id = ?1 OR c.status IN ('waiting', 'active'))
				ORDER BY (c.id = ?1) DESC LIMIT 1`).bind(id, user.id),
		]);
		const row = saved.results[0];
		if (!row) throw new HttpError(404, 'not_found', 'Nie znaleziono wyzwania.');
		return Response.json(fromRow(row));
	}
	const match = /^\/api\/competitions\/([^/]+)\/(accept|cancel|attempts)$/.exec(pathname);
	if (!match) return null;
	const [, id, action] = match;
	methods(request, [action === 'attempts' ? 'GET' : 'POST']);
	if (!uuidPattern.test(id)) throw new HttpError(400, 'invalid_input', 'Nieprawidłowy identyfikator wyzwania.');
	if (action === 'attempts') return Response.json(await listAttempts(env, id, user.id));
	if (request.body !== null) object(await readJson(request), []);
	const update = action === 'accept'
		? env.DB.prepare(`UPDATE competitions SET status = 'active', started_at = ?1, version = version + 1
			WHERE id = ?2 AND status = 'waiting' AND ?3 IN (participant_a_id, participant_b_id) AND created_by <> ?3`).bind(Date.now(), id, user.id)
		: env.DB.prepare(`UPDATE competitions SET status = 'cancelled', finished_at = ?1, version = version + 1
			WHERE id = ?2 AND status = 'waiting' AND ?3 IN (participant_a_id, participant_b_id)`).bind(Date.now(), id, user.id);
	const [, saved] = await env.DB.batch<CompetitionRow>([
		update,
		env.DB.prepare(`${select} WHERE c.id = ? AND ? IN (c.participant_a_id, c.participant_b_id)`).bind(id, user.id),
	]);
	const row = saved.results[0];
	if (!row) throw new HttpError(404, 'not_found', 'Nie znaleziono wyzwania.');
	if (action === 'accept' && row.createdBy === user.id) throw new HttpError(403, 'forbidden', 'Tylko zaproszona osoba może przyjąć wyzwanie.');
	if (action === 'accept' ? row.status !== 'active' && row.status !== 'finished' : row.status !== 'cancelled') {
		throw new HttpError(409, 'competition_conflict', 'Stan wyzwania już się zmienił. Odśwież wyzwanie.');
	}
	return Response.json(fromRow(row));
}
