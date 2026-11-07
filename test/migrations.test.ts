import { env } from 'cloudflare:workers';
import { applyD1Migrations, reset } from 'cloudflare:test';
import { expect, it } from 'vitest';
import { login, request } from './client';
import { USER_A, USER_B } from './fixtures';

it('gives open races the creator direction and keeps old races without one', async () => {
	await reset();
	await applyD1Migrations(env.DB, env.TEST_MIGRATIONS.filter(m => m.name !== '0011_competition_direction.sql'));
	await login(USER_A.id);
	const cookieB = await login(USER_B.id);
	expect((await request('/api/me', { method: 'PATCH', cookie: cookieB, body: { direction: 'pl-ru' } })).status).toBe(200);
	const insert = env.DB.prepare(`INSERT INTO competitions (id, participant_a_id, participant_b_id, target, created_by, status, created_at)
		VALUES (?, ?, ?, 20, ?, ?, 1)`);
	await env.DB.batch([
		insert.bind('33333333-3333-4333-8333-333333333333', USER_A.id, USER_B.id, USER_B.id, 'active'),
		insert.bind('55555555-5555-4555-8555-555555555555', USER_A.id, USER_B.id, USER_A.id, 'finished'),
	]);
	await applyD1Migrations(env.DB, env.TEST_MIGRATIONS);
	const { results } = await env.DB.prepare('SELECT id, direction FROM competitions ORDER BY id').all();
	expect(results).toEqual([
		{ id: '33333333-3333-4333-8333-333333333333', direction: 'pl-ru' },
		{ id: '55555555-5555-4555-8555-555555555555', direction: null },
	]);
});
