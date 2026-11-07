import { expect, test, type Page } from '@playwright/test';
import type { Card, Competition, Direction, ReviewResult } from '../shared/types';
import { CARD_B, USER_A, USER_B } from '../test/fixtures';
import { allCards, sql } from './cards';

let other: Page;
let cards: Card[];

async function signIn(page: Page, user = USER_A) {
	expect((await page.request.post('/api/session', { data: { username: user.username, password: user.password } })).status()).toBe(200);
	expect((await page.request.patch('/api/me', { data: { direction: 'ru-pl' } })).status()).toBe(200);
}

async function seed(page: Page): Promise<Card> {
	const response = await page.request.post('/api/cards', { data: { ...CARD_B, id: crypto.randomUUID() } });
	expect(response.status()).toBe(201);
	const card: Card = await response.json();
	cards.push(card);
	return card;
}

async function invite(page: Page, direction: Direction = 'ru-pl'): Promise<Competition> {
	const response = await page.request.post('/api/competitions', { data: { id: crypto.randomUUID(), direction } });
	expect(response.status()).toBe(200);
	return response.json();
}

async function start(page: Page, direction: Direction = 'ru-pl'): Promise<Competition> {
	const round = await invite(page, direction);
	const response = await other.request.post(`/api/competitions/${round.id}/accept`, { data: {} });
	expect(response.status()).toBe(200);
	return response.json();
}

async function current(page: Page): Promise<Competition> {
	return (await page.request.get('/api/competitions/current')).json();
}

async function openRace(page: Page, id: string) {
	await page.getByRole('button', { name: 'Zawody', exact: true }).click();
	await page.locator(`[data-race-id="${id}"] .race-open`).click();
}

async function train(page: Page, id: string) {
	await openRace(page, id);
	await page.getByRole('button', { name: 'Trenuj słówka', exact: true }).click();
}

function finish(round: Competition) {
	sql(`UPDATE competitions SET score_b = 20, status = 'finished', winner_id = '${USER_B.id}', finished_at = 1, version = version + 1 WHERE id = '${round.id}'`);
}

async function answer(page: Page) {
	await page.getByLabel('Odpowiedź po polsku').fill('Dom');
	await page.getByRole('button', { name: 'Sprawdź', exact: true }).click();
	await expect(page.locator('.study-feedback')).toContainText('Dobrze');
}

async function visibility(page: Page, value: 'hidden' | 'visible') {
	await page.evaluate(value => {
		Object.defineProperty(document, 'visibilityState', { configurable: true, value });
		document.dispatchEvent(new Event('visibilitychange'));
	}, value);
}

async function scores(page: Page, a: number, b: number, user = USER_A, main = false) {
	const block = page.getByTestId(main ? 'competition-scores' : 'competition-summary');
	await expect(block).toContainText(`${USER_A.displayName}${user.id === USER_A.id ? ' (Ty)' : ''}: ${a}`);
	await expect(block).toContainText(`${USER_B.displayName}${user.id === USER_B.id ? ' (Ty)' : ''}: ${b}`);
}

test.beforeEach(async ({ page, browser, baseURL }) => {
	cards = [];
	other = await (await browser.newContext({ baseURL, viewport: page.viewportSize() })).newPage();
	await page.context().setExtraHTTPHeaders({ Origin: baseURL!, 'CF-Connecting-IP': `competition-a-${crypto.randomUUID()}` });
	await other.context().setExtraHTTPHeaders({ Origin: baseURL!, 'CF-Connecting-IP': `competition-b-${crypto.randomUUID()}` });
	await signIn(page);
	await signIn(other, USER_B);
	for (const card of await allCards(page.request)) {
		expect((await page.request.patch(`/api/cards/${card.id}`, { data: { version: card.version, archived: true } })).status()).toBe(200);
	}
});

test.afterEach(async ({ page }) => {
	await page.close();
	await other.context().close();
	const ids = cards.map(card => `'${card.id}'`).join(',') || "''";
	sql(`DELETE FROM competition_events; DELETE FROM competitions;
		DELETE FROM reviews WHERE json_extract(input_json, '$.cardId') IN (${ids});
		DELETE FROM card_progress WHERE card_id IN (${ids}); DELETE FROM cards WHERE id IN (${ids})`);
});

test("shows another learner's invitation and saved score", async ({ page }) => {
	await seed(page);
	await other.goto('/');
	await other.getByRole('button', { name: 'Zawody', exact: true }).click();
	await expect(other.getByRole('button', { name: 'Nowy wyścig', exact: true })).toBeEnabled();
	const round = await invite(page);
	await expect(other.locator(`[data-race-id="${round.id}"]`)).toBeVisible({ timeout: 7000 });
	await other.locator(`[data-race-id="${round.id}"] .race-open`).click();
	await other.getByRole('button', { name: 'Przyjmij wyzwanie', exact: true }).click();
	await expect(other.getByRole('button', { name: 'Trenuj słówka', exact: true })).toBeEnabled();
	await page.goto('/');
	await train(page, round.id);
	await scores(page, 0, 0);
	await answer(page);
	await expect(other.getByTestId('competition-scores')).toContainText(`${USER_A.displayName}: 1`, { timeout: 7000 });
	await scores(other, 1, 0, USER_B, true);
	expect((await current(page)).participants.map(p => p.score)).toEqual([1, 0]);
});

test('shares one snapshot across tabs', async ({ page }) => {
	await seed(page);
	const round = await start(page);
	await page.clock.install();
	let reads = 0;
	page.on('request', request => { if (request.url().endsWith('/api/competitions/current')) reads++; });
	await page.goto('/');
	await train(page, round.id);
	await scores(page, 0, 0);
	await answer(page);
	await scores(page, 1, 0);
	const before = reads;
	for (const screen of [{ name: 'Słówka' }, { name: 'Dodaj' }, { name: 'Nauka' }]) {
		await page.getByRole('button', { name: screen.name, exact: true }).click();
		await expect(page.getByTestId('competition-summary')).toHaveCount(0);
		await expect(page.locator('.tabs .tab-dot')).toHaveAttribute('data-state', 'active');
	}
	await openRace(page, round.id);
	await scores(page, 1, 0, USER_A, true);
	expect(reads).toBe(before);
	await page.clock.runFor(5000);
	await expect.poll(() => reads).toBe(before + 1);
	await scores(page, 1, 0, USER_A, true);
	await page.clock.runFor(5000);
	await expect.poll(() => reads).toBe(before + 2);
	await other.goto('/');
	await openRace(other, round.id);
	await scores(other, 1, 0, USER_B, true);
});

test('keeps the race ID when an editable review retry creates a new attempt', async ({ page }) => {
	await seed(page);
	const round = await start(page);
	await page.goto('/');
	await train(page, round.id);
	await scores(page, 0, 0);
	await page.getByLabel('Odpowiedź po polsku').fill('ошибка подключения');
	const failed = page.waitForResponse(response => response.url().endsWith('/api/reviews'));
	await page.getByRole('button', { name: 'Sprawdź', exact: true }).click();
	const rejected = await failed;
	expect(rejected.status()).toBe(503);
	expect(rejected.headers()['x-review-recovery']).toBe('editable');
	const original = rejected.request().postDataJSON();
	expect(original.competitionId).toBe(round.id);
	await expect(page.getByLabel('Odpowiedź po polsku')).toBeEnabled();
	await page.getByLabel('Odpowiedź po polsku').fill('Dom');
	const saved = page.waitForResponse(response => response.url().endsWith('/api/reviews'));
	await page.getByRole('button', { name: 'Ponów', exact: true }).click();
	const response = await saved;
	expect(response.status()).toBe(200);
	const retry = response.request().postDataJSON();
	expect(retry.attemptId).not.toBe(original.attemptId);
	expect(retry.competitionId).toBe(round.id);
	expect((await response.json()).competition).toMatchObject({ competitionId: round.id, delta: 1, scoreAfter: 1 });
	await expect(page.locator('.study-feedback')).toContainText('Wynik: 1.');
	await scores(page, 1, 0);
});

for (const scenario of [
	{ name: 'a tie at zero', a: 0, b: 0, text: 'Remis.' },
	{ name: 'a tie above zero', a: 8, b: 8, text: 'Remis.' },
	{ name: 'the first learner leads by one', a: 9, b: 8, text: `${USER_A.displayName} prowadzi o 1 punkt.` },
	{ name: 'the second learner leads by four', a: 8, b: 12, text: `${USER_B.displayName} prowadzi o 4 punkty.` },
	{ name: 'a twelve-point lead', a: 12, b: 0, text: `${USER_A.displayName} prowadzi o 12 punktów.` },
]) test(`shows the active result in both score displays for ${scenario.name}`, async ({ page }) => {
	const round = await start(page);
	sql(`UPDATE competitions SET score_a = ${scenario.a}, score_b = ${scenario.b}, version = version + 1 WHERE id = '${round.id}'`);
	await page.goto('/');
	await train(page, round.id);
	await expect(page.getByTestId('competition-summary')).toContainText(scenario.text);
	await scores(page, scenario.a, scenario.b);
	await page.getByTestId('competition-summary').click();
	await expect(page.getByTestId('competition-scores')).toContainText(scenario.text);
	await scores(page, scenario.a, scenario.b, USER_A, true);
});

test('retries a lost review against the same round after reload', async ({ page }) => {
	await seed(page);
	const round = await start(page);
	let originalBody = '';
	await page.route('**/api/reviews', async route => {
		originalBody = route.request().postData()!;
		expect((await route.fetch()).status()).toBe(200);
		await route.abort();
	});
	await page.goto('/');
	await train(page, round.id);
	await scores(page, 0, 0);
	await page.getByLabel('Odpowiedź po polsku').fill('Dom');
	await page.getByRole('button', { name: 'Sprawdź', exact: true }).click();
	await expect(page.getByRole('alert')).toContainText('nie został potwierdzony');
	expect(JSON.parse(originalBody).competitionId).toBe(round.id);
	await page.unroute('**/api/reviews');
	await page.reload();
	await expect(page.getByText('Na dziś wszystko gotowe.', { exact: true })).toBeVisible();
	await expect(page.getByRole('alert')).toHaveCount(0);
	await train(page, round.id);
	await expect(page.getByRole('alert')).toContainText('nie został potwierdzony');
	const retry = page.waitForRequest(request => request.url().endsWith('/api/reviews'));
	await page.getByRole('button', { name: 'Ponów', exact: true }).click();
	expect((await retry).postData()).toBe(originalBody);
	await expect(page.locator('.study-feedback')).toContainText('Wynik: 1.');
	await scores(page, 1, 0);
	expect((await current(page)).participants.map(p => p.score)).toEqual([1, 0]);
});

test('sends no race ID from Nauka during a race', async ({ page }) => {
	await seed(page);
	const round = await start(page);
	await page.goto('/');
	const saved = page.waitForResponse(response => response.url().endsWith('/api/reviews'));
	await answer(page);
	const response = await saved;
	expect(response.request().postDataJSON().competitionId).toBeUndefined();
	expect((await response.json()).competition).toBeNull();
	await expect(page.locator('.study-feedback')).not.toContainText('Wynik:');
	await openRace(page, round.id);
	await scores(page, 0, 0, USER_A, true);
	expect((await current(page)).participants.map(p => p.score)).toEqual([0, 0]);
});

test('shows the fixed race direction in race training', async ({ page }) => {
	await seed(page);
	const round = await start(page, 'pl-ru');
	await page.goto('/');
	await train(page, round.id);
	await expect(page.getByText('Polski → Rosyjski', { exact: true })).toBeVisible();
	await expect(page.getByText('Kierunek wyścigu. Nie zmienia się do końca wyścigu.', { exact: true })).toBeVisible();
	await expect(page.getByRole('button', { name: 'RU -> PL', exact: true })).toHaveCount(0);
	await expect(page.getByRole('button', { name: 'PL -> RU', exact: true })).toHaveCount(0);
	await page.getByLabel('Odpowiedź po rosyjsku').fill('Дом');
	const saved = page.waitForRequest(request => request.url().endsWith('/api/reviews'));
	await page.getByRole('button', { name: 'Sprawdź', exact: true }).click();
	expect((await saved).postDataJSON()).toMatchObject({ direction: 'pl-ru', competitionId: round.id });
	await expect(page.locator('.study-feedback')).toContainText('Wynik: 1.');
	await page.getByRole('button', { name: 'Nauka', exact: true }).click();
	await expect(page.getByRole('button', { name: 'RU -> PL', exact: true })).toHaveAttribute('aria-pressed', 'true');
});

for (const scenario of [
	{ name: 'a typed answer is on screen', typed: true, restart: false, focused: 'Zagraj jeszcze raz' },
	{ name: 'the other learner restarts', typed: false, restart: true, focused: 'Wróć do wyścigu' },
]) test(`ends race training after the finish when ${scenario.name}`, async ({ page }) => {
	await seed(page);
	const round = await start(page);
	await page.clock.install();
	await page.goto('/');
	await train(page, round.id);
	const field = page.getByLabel('Odpowiedź po polsku');
	await expect(field).toBeEnabled();
	if (scenario.typed) await field.fill('Dom');
	finish(round);
	const rematch = scenario.restart ? await invite(other) : null;
	const polled = page.waitForResponse(response => response.url().endsWith('/api/competitions/current'));
	await page.clock.runFor(5000);
	await polled;
	if (!rematch) await expect(page.getByTestId('competition-summary')).toContainText(`Wygrywa ${USER_B.displayName}!`);
	await expect(page.getByTestId('study-term')).toHaveText('Дом');
	if (scenario.typed) await expect(field).toHaveValue('Dom');
	else await field.fill('Dom');
	const saved = page.waitForResponse(response => response.url().endsWith('/api/reviews'));
	await page.getByRole('button', { name: 'Sprawdź', exact: true }).click();
	const response = await saved;
	expect(response.status()).toBe(200);
	expect(response.request().postDataJSON().competitionId).toBe(round.id);
	await expect(page.locator('.study-feedback')).toContainText('Wyścig już się zakończył. Ta odpowiedź nie liczy się do wyniku.');
	await page.getByRole('button', { name: 'Dalej', exact: true }).click();
	await expect(page.getByRole('button', { name: 'Wróć do wyścigu', exact: true })).toBeEnabled();
	await expect(page.getByLabel('Odpowiedź po polsku')).toHaveCount(0);
	await expect(page.locator('.race-result')).toContainText(`Wygrywa ${USER_B.displayName}!`);
	const restart = page.getByRole('button', { name: 'Zagraj jeszcze raz', exact: true });
	if (rematch) await expect(restart).toBeDisabled();
	else await expect(restart).toBeEnabled();
	await expect(page.getByRole('button', { name: scenario.focused, exact: true })).toBeFocused();
	if (rematch) {
		const next = await current(page);
		expect(next.id).toBe(rematch.id);
		expect(next.participants.map(p => p.score)).toEqual([0, 0]);
	}
	await page.getByRole('button', { name: 'Wróć do wyścigu', exact: true }).click();
	await expect(page.getByRole('heading', { name: 'Wyścig do 20' })).toBeVisible();
});

test('removes the draft of a finished race', async ({ page }) => {
	await seed(page);
	const round = await start(page);
	await page.clock.install();
	await page.goto('/');
	await train(page, round.id);
	await page.getByLabel('Odpowiedź po polsku').fill('Dom');
	expect(await page.evaluate(() => Object.keys(sessionStorage))).toContain(`race-draft:${round.id}:${USER_A.id}`);
	await page.getByTestId('competition-summary').click();
	finish(round);
	await page.clock.runFor(5000);
	await expect(page.getByTestId('competition-scores')).toContainText(`Wygrywa ${USER_B.displayName}!`);
	await expect.poll(() => page.evaluate(() => Object.keys(sessionStorage).filter(key => key.startsWith('race-draft:')))).toEqual([]);
	const next = await start(page);
	await page.clock.runFor(5000);
	await train(page, next.id);
	await expect(page.getByLabel('Odpowiedź po polsku')).toBeEnabled();
	await expect(page.getByLabel('Odpowiedź po polsku')).toHaveValue('');
});

for (const scenario of [
	{ name: 'loading', fail: false },
	{ name: 'read error', fail: true },
]) test(`ignores a previous account's late response during ${scenario.name}`, async ({ page }) => {
	const round = await invite(page);
	await page.clock.install();
	const received = Promise.withResolvers<void>(), release = Promise.withResolvers<void>(), finished = Promise.withResolvers<void>();
	const newRead = Promise.withResolvers<void>(), releaseNew = Promise.withResolvers<void>();
	let reads = 0;
	await page.route('**/api/competitions/current', async route => {
		if (++reads === 2) {
			const response = await route.fetch();
			received.resolve();
			await release.promise;
			await route.fulfill({ response });
			finished.resolve();
		} else if (reads === 3) {
			newRead.resolve();
			if (scenario.fail) await route.fulfill({ status: 503, json: { error: 'Nowe konto: brak połączenia.' } });
			else { await releaseNew.promise; await route.continue(); }
		} else await route.continue();
	});
	await page.goto('/');
	await expect(page.locator('.tabs .tab-dot')).toHaveAttribute('data-state', 'waiting');
	await page.clock.runFor(5000);
	await received.promise;
	try {
		await page.getByRole('button', { name: 'Wyloguj', exact: true }).click();
		expect((await other.request.post(`/api/competitions/${round.id}/accept`, { data: {} })).status()).toBe(200);
		await page.getByLabel('Nazwa użytkownika').fill(USER_B.username);
		await page.getByLabel('Hasło', { exact: true }).fill(USER_B.password);
		await page.getByRole('button', { name: 'Zaloguj', exact: true }).click();
		await newRead.promise;
		await page.getByRole('button', { name: 'Zawody', exact: true }).click();
		if (scenario.fail) await expect(page.getByRole('alert')).toContainText('Nowe konto: brak połączenia.');
		release.resolve();
		await finished.promise;
		await expect(page.locator('.tabs .tab-dot')).toHaveCount(0);
		if (scenario.fail) {
			await expect(page.getByRole('alert')).toContainText('Nowe konto: brak połączenia.');
			await page.getByRole('button', { name: 'Ponów', exact: true }).click();
		}
	} finally { release.resolve(); releaseNew.resolve(); }
	await page.locator(`[data-race-id="${round.id}"] .race-open`).click();
	await scores(page, 0, 0, USER_B, true);
	await expect(page.getByRole('button', { name: 'Trenuj słówka', exact: true })).toBeEnabled();
	await expect(page.getByRole('button', { name: 'Przyjmij wyzwanie', exact: true })).toHaveCount(0);
});

for (const scenario of [
	{ name: 'an active round', active: true },
	{ name: 'no round', active: false },
]) test(`polls only while visible with ${scenario.name}`, async ({ page }) => {
	if (scenario.active) await start(page);
	await page.clock.install();
	let readCount = 0;
	page.on('request', request => { if (request.url().endsWith('/api/competitions/current')) readCount++; });
	await page.goto('/');
	await page.getByRole('button', { name: 'Zawody', exact: true }).click();
	if (scenario.active) await expect(page.locator('.tabs .tab-dot')).toHaveAttribute('data-state', 'active');
	else await expect(page.getByRole('button', { name: 'Nowy wyścig', exact: true })).toBeEnabled();
	await visibility(page, 'hidden');
	const before = readCount;
	await page.clock.runFor(15000);
	expect(readCount).toBe(before);
	await visibility(page, 'visible');
	await expect.poll(() => readCount).toBe(before + 1);
});

test('keeps answer feedback after a competition read failure', async ({ page }) => {
	await seed(page);
	const round = await start(page);
	await page.clock.install();
	await page.goto('/');
	await train(page, round.id);
	await scores(page, 0, 0);
	await page.route('**/api/competitions/current', route => route.fulfill({ status: 503, json: { error: 'Brak wyniku.' } }));
	await answer(page);
	await expect(page.locator('.study-feedback')).toContainText('Wynik: 1.');
	await expect(page.getByTestId('competition-summary')).toContainText('Wynik może być nieaktualny');
	await scores(page, 0, 0);
	await page.getByTestId('competition-summary').click();
	await expect(page.getByRole('alert')).toContainText('Brak wyniku.');
	await page.unroute('**/api/competitions/current');
	await expect(page.getByRole('button', { name: 'Odśwież', exact: true })).toBeEnabled();
	await page.getByRole('button', { name: 'Odśwież', exact: true }).click();
	await scores(page, 1, 0, USER_A, true);
	await expect(page.getByText('Wynik może być nieaktualny', { exact: true })).toHaveCount(0);
});

test('recovers an initial competition read', async ({ page }) => {
	await seed(page);
	const round = await invite(other);
	await page.clock.install();
	await page.route('**/api/competitions/current', route => route.fulfill({ status: 503, json: { error: 'Brak wyzwania.' } }));
	await page.goto('/');
	await expect(page.getByLabel('Odpowiedź po polsku')).toBeEnabled();
	await page.getByRole('button', { name: 'Zawody', exact: true }).click();
	await expect(page.getByRole('alert')).toContainText('Brak wyzwania.');
	await expect(page.getByRole('button', { name: 'Nowy wyścig', exact: true })).toBeDisabled();
	await page.unroute('**/api/competitions/current');
	await page.getByRole('button', { name: 'Ponów', exact: true }).click();
	await expect(page.locator('.tabs .tab-dot')).toHaveAttribute('data-state', 'waiting');
	await page.locator(`[data-race-id="${round.id}"] .race-open`).click();
	await scores(page, 0, 0, USER_A, true);
	await expect(page.getByRole('button', { name: 'Przyjmij wyzwanie', exact: true })).toBeEnabled();
});

test('retries the same invitation UUID', async ({ page }) => {
	await page.clock.install();
	await page.goto('/');
	await page.getByRole('button', { name: 'Zawody', exact: true }).click();
	await page.getByRole('button', { name: 'Nowy wyścig', exact: true }).click();
	await page.getByRole('button', { name: 'PL -> RU', exact: true }).click();
	let originalBody = '';
	await page.route('**/api/competitions', async route => {
		originalBody = route.request().postData()!;
		expect((await route.fetch()).status()).toBe(200);
		await route.abort();
	});
	await page.route('**/api/competitions/current', route => route.fulfill({ status: 503, json: { error: 'Brak wyniku.' } }));
	await page.getByRole('button', { name: 'Zaproś do wyścigu', exact: true }).click();
	await expect(page.getByRole('alert').filter({ hasText: 'Nie udało się zapisać wyzwania' })).toBeVisible();
	expect(JSON.parse(originalBody).direction).toBe('pl-ru');
	await page.unroute('**/api/competitions');
	await page.unroute('**/api/competitions/current');
	const retry = page.waitForRequest(request => request.url().endsWith('/api/competitions') && request.method() === 'POST');
	await page.getByRole('button', { name: 'Ponów zaproszenie', exact: true }).click();
	expect((await retry).postData()).toBe(originalBody);
	await expect(page.getByRole('button', { name: 'Anuluj zaproszenie', exact: true })).toBeEnabled();
	await expect(page.getByText('Polski → Rosyjski', { exact: true })).toBeVisible();
	await expect(page.getByRole('button', { name: 'Przyjmij wyzwanie', exact: true })).toHaveCount(0);
	expect((await current(page)).id).toBe(JSON.parse(originalBody).id);
	sql('CREATE TABLE one_round (n INTEGER CHECK (n = 1)); INSERT INTO one_round SELECT count(*) FROM competitions; DROP TABLE one_round;');
});

for (const scenario of [
	{ name: 'the creator cancels the first invitation', invited: false, history: false },
	{ name: 'the invited learner cancels a rematch', invited: true, history: true },
]) test(`selects the current round and stays busy until refresh when ${scenario.name}`, async ({ page }) => {
	let finished: Competition | null = null;
	if (scenario.history) {
		finished = await start(page);
		sql(`UPDATE competitions SET score_a = 20, status = 'finished', winner_id = '${USER_A.id}', finished_at = 1, version = version + 1 WHERE id = '${finished.id}'`);
	}
	const round = await invite(scenario.invited ? other : page);
	await page.goto('/');
	await openRace(page, round.id);
	await expect(page.getByRole('button', { name: 'Anuluj zaproszenie', exact: true })).toBeEnabled();
	const received = Promise.withResolvers<void>(), release = Promise.withResolvers<void>();
	await page.route('**/api/competitions/current', async route => {
		received.resolve();
		await release.promise;
		await route.continue();
	});
	await page.getByRole('button', { name: 'Anuluj zaproszenie', exact: true }).click();
	await received.promise;
	try {
		await expect(page.getByRole('button', { name: 'Anuluj zaproszenie', exact: true })).toBeDisabled();
		await expect(page.getByRole('navigation').locator('button:disabled')).toHaveCount(4);
		await expect(page.getByRole('button', { name: 'Wyloguj', exact: true })).toBeDisabled();
	} finally { release.resolve(); }
	await expect(page.getByRole('heading', { name: 'Zawody', exact: true })).toBeVisible();
	await expect(page.getByRole('button', { name: 'Nowy wyścig', exact: true })).toBeEnabled();
	await expect(page.getByRole('button', { name: 'Anuluj zaproszenie', exact: true })).toHaveCount(0);
	if (finished) {
		expect((await current(page)).id).toBe(finished.id);
		await openRace(page, finished.id);
		await scores(page, 20, 0, USER_A, true);
		await expect(page.getByRole('button', { name: 'Zagraj jeszcze raz', exact: true })).toBeEnabled();
	} else expect(await current(page)).toBeNull();
});

test('locks navigation while a review is saved', async ({ page }) => {
	await seed(page);
	const round = await start(page);
	const received = Promise.withResolvers<void>(), release = Promise.withResolvers<void>();
	await page.route('**/api/reviews', async route => { received.resolve(); await release.promise; await route.continue(); });
	await page.goto('/');
	await train(page, round.id);
	await scores(page, 0, 0);
	await page.getByLabel('Odpowiedź po polsku').fill('Dom');
	await page.getByRole('button', { name: 'Sprawdź', exact: true }).click();
	await received.promise;
	try {
		await expect(page.getByTestId('competition-summary')).toBeDisabled();
		await expect(page.getByRole('navigation').getByRole('button')).toHaveCount(4);
		await expect(page.getByRole('navigation').locator('button:disabled')).toHaveCount(4);
		await expect(page.getByRole('button', { name: 'Wyloguj', exact: true })).toBeDisabled();
	} finally { release.resolve(); }
	await expect(page.locator('.study-feedback')).toContainText('Dobrze');
	await expect(page.getByTestId('competition-summary')).toBeEnabled();
	await expect(page.getByRole('navigation').locator('button:disabled')).toHaveCount(0);
});

for (const scenario of [
	{ name: 'shows the saved zero-floor contribution', score: 0, action: 'reveal', delta: 0, after: 0, text: 'Zostajesz na starcie. Wynik: 0.' },
	{ name: 'shows the saved downward contribution', score: 1, action: 'reveal', delta: -1, after: 0, text: 'Cofasz się o 1. Wynik: 0.' },
	{ name: 'keeps the winning contribution after the round finishes', score: 19, action: 'answer', delta: 1, after: 20, text: 'Idziesz w górę o 1. Wynik: 20.' },
]) test(scenario.name, async ({ page }) => {
	await seed(page);
	const round = await start(page);
	if (scenario.score) sql(`UPDATE competitions SET score_a = ${scenario.score}, version = version + 1 WHERE id = '${round.id}'`);
	await page.goto('/');
	await train(page, round.id);
	await scores(page, scenario.score, 0);
	const saved = page.waitForResponse(response => response.url().endsWith('/api/reviews'));
	if (scenario.action === 'answer') await answer(page);
	else await page.getByRole('button', { name: 'Pokaż odpowiedź', exact: true }).click();
	const result: ReviewResult = await (await saved).json();
	expect(result.competition).toMatchObject({ competitionId: round.id, delta: scenario.delta, scoreAfter: scenario.after });
	await expect(page.locator('.study-feedback')).toContainText(scenario.text);
	await scores(page, scenario.after, 0);
	if (scenario.after === 20) {
		await expect(page.getByTestId('competition-summary')).toContainText(`Wygrywa ${USER_A.displayName}!`);
		await page.getByTestId('competition-summary').click();
		await expect(page.getByRole('button', { name: 'Zagraj jeszcze raz', exact: true })).toBeEnabled();
		await page.getByRole('button', { name: 'Zagraj jeszcze raz', exact: true }).click();
		await expect(page.getByRole('button', { name: 'Anuluj zaproszenie', exact: true })).toBeEnabled();
		await scores(page, 0, 0, USER_A, true);
	} else {
		await page.getByRole('button', { name: 'Dalej', exact: true }).click();
		await expect(page.getByText(/Następna powtórka:/)).toBeVisible();
	}
});

test('hides race training for an active race without a direction', async ({ page }) => {
	const round = await start(page);
	sql(`UPDATE competitions SET direction = NULL WHERE id = '${round.id}'`);
	await page.goto('/');
	await openRace(page, round.id);
	await expect(page.getByTestId('competition-scores')).toContainText('Remis.');
	await expect(page.getByRole('button', { name: 'Trenuj słówka', exact: true })).toHaveCount(0);
});

for (const scenario of [
	{ name: 'no race', setup: 'none', state: null },
	{ name: 'waiting invitation', setup: 'invite', state: 'waiting' },
	{ name: 'active race', setup: 'start', state: 'active' },
	{ name: 'finished race', setup: 'finish', state: null },
	{ name: 'cancelled invitation', setup: 'cancel', state: null },
] satisfies { name: string; setup: 'none' | 'invite' | 'start' | 'finish' | 'cancel'; state: 'waiting' | 'active' | null }[]) test(`shows the race state on the Zawody tab: ${scenario.name}`, async ({ page }) => {
	if (scenario.setup === 'invite') await invite(page);
	if (scenario.setup === 'start') await start(page);
	if (scenario.setup === 'finish') finish(await start(page));
	if (scenario.setup === 'cancel') {
		const round = await invite(page);
		expect((await other.request.post(`/api/competitions/${round.id}/cancel`, { data: {} })).status()).toBe(200);
	}
	for (const viewer of [page, other]) {
		await viewer.goto('/');
		const tab = viewer.getByRole('button', { name: 'Zawody', exact: true });
		if (scenario.state) {
			await expect(tab.locator('.tab-dot')).toHaveAttribute('data-state', scenario.state);
			await expect(tab).toHaveAccessibleDescription(scenario.state === 'waiting' ? 'Zaproszenie czeka na odpowiedź.' : 'Wyścig trwa.');
		} else {
			await tab.click();
			await expect(viewer.getByRole('button', { name: 'Nowy wyścig', exact: true })).toBeEnabled();
			await expect(tab.locator('.tab-dot')).toHaveCount(0);
			await expect(tab).toHaveAccessibleDescription('');
		}
	}
});

for (const scenario of [
	{ name: 'list row', place: 'row' },
	{ name: 'race view', place: 'race' },
] satisfies { name: string; place: 'row' | 'race' }[]) test(`restarts a finished race from the ${scenario.name}`, async ({ page }) => {
	const round = await start(page, 'pl-ru');
	finish(round);
	await other.goto('/');
	await other.getByRole('button', { name: 'Zawody', exact: true }).click();
	if (scenario.place === 'race') await other.locator(`[data-race-id="${round.id}"] .race-open`).click();
	const restart = scenario.place === 'row' ? other.locator(`[data-race-id="${round.id}"] .race-restart`) : other.getByRole('button', { name: 'Zagraj jeszcze raz', exact: true });
	await expect(restart).toBeEnabled();
	const posted = other.waitForRequest(request => request.url().endsWith('/api/competitions') && request.method() === 'POST');
	await restart.click();
	expect((await posted).postDataJSON().direction).toBe('pl-ru');
	await expect(other.getByRole('button', { name: 'Anuluj zaproszenie', exact: true })).toBeEnabled();
	await expect(other.getByRole('heading', { name: 'Wyścig do 20' })).toBeVisible();
	await expect(other.getByTestId('competition-scores')).toContainText('Oczekiwanie na przyjęcie wyzwania');
	const next = await current(other);
	expect(next).toMatchObject({ status: 'waiting', direction: 'pl-ru', createdBy: USER_B.id });
	expect(next.id).not.toBe(round.id);
});

test('disables the restart button while a race is open', async ({ page }) => {
	const old = await start(page);
	finish(old);
	await invite(other);
	await page.goto('/');
	await page.getByRole('button', { name: 'Zawody', exact: true }).click();
	await expect(page.locator('.tabs .tab-dot')).toHaveAttribute('data-state', 'waiting');
	await expect(page.locator(`[data-race-id="${old.id}"] .race-restart`)).toBeDisabled();
	await expect(page.getByRole('button', { name: 'Nowy wyścig', exact: true })).toBeDisabled();
	await page.locator(`[data-race-id="${old.id}"] .race-open`).click();
	await expect(page.getByRole('button', { name: 'Zagraj jeszcze raz', exact: true })).toBeDisabled();
});

test('opens the new race form for an old race without a direction', async ({ page }) => {
	const old = await start(page);
	finish(old);
	sql(`UPDATE competitions SET direction = NULL WHERE id = '${old.id}'`);
	await page.goto('/');
	await page.getByRole('button', { name: 'Zawody', exact: true }).click();
	const row = page.locator(`[data-race-id="${old.id}"]`);
	await expect(row).toContainText(`Wygrywa ${USER_B.displayName}`);
	await expect(row).not.toContainText('→');
	await row.locator('.race-restart').click();
	await expect(page.getByRole('heading', { name: 'Nowy wyścig', exact: true })).toBeVisible();
	await expect(page.getByRole('button', { name: 'RU -> PL', exact: true })).toHaveAttribute('aria-pressed', 'true');
});

test("shows the inviter's direction to the invited learner", async ({ page }) => {
	await page.goto('/');
	await page.getByRole('button', { name: 'Zawody', exact: true }).click();
	await page.getByRole('button', { name: 'Nowy wyścig', exact: true }).click();
	await page.getByRole('button', { name: 'PL -> RU', exact: true }).click();
	await page.getByRole('button', { name: 'Zaproś do wyścigu', exact: true }).click();
	await expect(page.getByRole('button', { name: 'Anuluj zaproszenie', exact: true })).toBeEnabled();
	const round = await current(page);
	expect(round.direction).toBe('pl-ru');
	await other.goto('/');
	await other.getByRole('button', { name: 'Zawody', exact: true }).click();
	await expect(other.locator(`[data-race-id="${round.id}"]`)).toContainText('PL → RU');
	await other.locator(`[data-race-id="${round.id}"] .race-open`).click();
	await expect(other.getByText('Polski → Rosyjski', { exact: true })).toBeVisible();
	await expect(other.getByRole('button', { name: 'Przyjmij wyzwanie', exact: true })).toBeEnabled();
});

test('shows the returned direction after concurrent invitations', async ({ page }) => {
	await page.goto('/');
	await page.getByRole('button', { name: 'Zawody', exact: true }).click();
	await page.getByRole('button', { name: 'Nowy wyścig', exact: true }).click();
	await page.getByRole('button', { name: 'PL -> RU', exact: true }).click();
	const round = await invite(other, 'ru-pl');
	await page.getByRole('button', { name: 'Zaproś do wyścigu', exact: true }).click();
	await expect(page.getByRole('heading', { name: 'Wyścig do 20' })).toBeVisible();
	await expect(page.getByText('Rosyjski → Polski', { exact: true })).toBeVisible();
	await expect(page.getByRole('button', { name: 'Przyjmij wyzwanie', exact: true })).toBeEnabled();
	expect((await current(page)).id).toBe(round.id);
});

test('returns to the list when the other learner cancels the race on screen', async ({ page }) => {
	const round = await invite(other);
	await page.clock.install();
	await page.goto('/');
	await openRace(page, round.id);
	await expect(page.getByRole('button', { name: 'Przyjmij wyzwanie', exact: true })).toBeEnabled();
	expect((await other.request.post(`/api/competitions/${round.id}/cancel`, { data: {} })).status()).toBe(200);
	await page.clock.runFor(5000);
	await expect(page.getByRole('heading', { name: 'Zawody', exact: true })).toBeVisible();
	await expect(page.getByRole('button', { name: 'Wróć do listy wyścigów', exact: true })).toHaveCount(0);
	await expect(page.getByTestId('race-row')).toHaveCount(0);
	await expect(page.getByText('Nie ma jeszcze wyścigów.', { exact: true })).toBeVisible();
});

test('pages the race list', async ({ page }) => {
	const card = await seed(page);
	const day = 86_400_000, first = Date.UTC(2026, 9, 1, 12);
	// Twelve races, one a day at 12:00 UTC: eleven finished races and the newest race active.
	const races = Array.from({ length: 12 }, (_, index) => ({ id: crypto.randomUUID(), createdAt: first + index * day, active: index === 11 }));
	sql(`INSERT INTO competitions (id, participant_a_id, participant_b_id, target, created_by, direction, status, score_a, winner_id, created_at, started_at, finished_at)
		VALUES ${races.map(race => `('${race.id}', '${USER_A.id}', '${USER_B.id}', 20, '${USER_A.id}', 'ru-pl', '${race.active ? 'active' : 'finished'}', ${race.active ? 0 : 20},
			${race.active ? 'NULL' : `'${USER_A.id}'`}, ${race.createdAt}, ${race.createdAt}, ${race.active ? 'NULL' : race.createdAt})`).join(', ')}`);
	const active = races[11], oldest = races[0], marked = races[10];
	await page.clock.install();
	await page.goto('/');
	await page.getByRole('button', { name: 'Zawody', exact: true }).click();
	const rows = page.getByTestId('race-row');
	await expect(rows).toHaveCount(10);
	await expect(rows.first()).toHaveAttribute('data-race-id', active.id);
	await expect(rows.first()).toContainText(await page.evaluate(time => new Date(time).toLocaleDateString('pl-PL'), active.createdAt));
	await page.getByRole('button', { name: 'Więcej wyścigów', exact: true }).click();
	await expect(rows).toHaveCount(12);
	await expect(page.getByRole('button', { name: 'Więcej wyścigów', exact: true })).toHaveCount(0);
	await page.locator(`[data-race-id="${oldest.id}"] .race-open`).click();
	const back = page.getByRole('button', { name: 'Wróć do listy wyścigów', exact: true });
	await expect(back).toBeVisible();
	// Only the next list read shows this score, so it marks that the read reached the screen.
	sql(`UPDATE competitions SET score_b = 5, version = version + 1 WHERE id = '${marked.id}'`);
	const saved = await other.request.post('/api/reviews', { data: { attemptId: crypto.randomUUID(), cardId: card.id, cardVersion: card.version,
		progressVersion: 0, direction: 'ru-pl', competitionId: active.id, action: 'answer', answer: 'Dom' } });
	expect(saved.status()).toBe(200);
	expect((await saved.json()).competition).toMatchObject({ competitionId: active.id, delta: 1, scoreAfter: 1 });
	const reread = page.waitForResponse(response => response.url().endsWith('/api/competitions') && response.request().method() === 'GET');
	await page.clock.runFor(5000);
	await reread;
	await expect(back).toBeVisible();
	await back.click();
	await expect(page.locator(`[data-race-id="${marked.id}"]`)).toContainText(`${USER_A.displayName} 20 : 5 ${USER_B.displayName}`);
	await expect(rows.first()).toContainText(`${USER_A.displayName} 0 : 1 ${USER_B.displayName}`);
	await expect(rows).toHaveCount(12);
	await expect(page.getByRole('button', { name: 'Więcej wyścigów', exact: true })).toHaveCount(0);
});

test('retries a failed race list read', async ({ page }) => {
	// Only the list returns the finished race. The polled round is the newer invitation.
	const old = await start(page);
	finish(old);
	await invite(other);
	await page.route('**/api/competitions', route => route.abort(), { times: 1 });
	await page.goto('/');
	await expect(page.locator('.tabs .tab-dot')).toHaveAttribute('data-state', 'waiting');
	await page.getByRole('button', { name: 'Zawody', exact: true }).click();
	await expect(page.getByRole('alert')).toContainText('Nie udało się wczytać wyścigów. Sprawdź połączenie i spróbuj ponownie.');
	await expect(page.locator(`[data-race-id="${old.id}"]`)).toHaveCount(0);
	await page.getByRole('button', { name: 'Ponów', exact: true }).click();
	await expect(page.locator(`[data-race-id="${old.id}"]`)).toContainText('Zakończony');
	await expect(page.getByRole('alert')).toHaveCount(0);
});

async function review(page: Page, card: Card, round: Competition, input: object) {
	const response = await page.request.post('/api/reviews', { data: { attemptId: crypto.randomUUID(), cardId: card.id, cardVersion: card.version, competitionId: round.id, ...input } });
	expect(response.status()).toBe(200);
}

test('shows the answer, the correction and the explanation of an attempt', async ({ page }) => {
	const card = await seed(page);
	const round = await start(page);
	await page.goto('/');
	await train(page, round.id);
	await answer(page);
	await review(other, card, round, { progressVersion: 0, direction: 'ru-pl', action: 'reveal' });
	await page.getByTestId('competition-summary').click();
	await expect(page.getByRole('heading', { name: 'Próby' })).toBeVisible();
	await expect(page.getByRole('heading', { name: 'Dom / Дом' })).toBeVisible();
	const first = page.getByText(`${USER_A.displayName}: Dobrze`, { exact: true });
	await expect(page.getByText(`${USER_B.displayName}: Pokazano odpowiedź`, { exact: true })).toBeVisible();
	await expect(page.getByText('Poprawna odpowiedź: Dom').first()).toBeHidden();
	await first.click();
	await expect(page.getByText('Odpowiedź: Dom', { exact: true })).toBeVisible();
	await expect(page.getByText('Poprawna odpowiedź: Dom', { exact: true }).first()).toBeVisible();
	await expect(page.getByText('Dobrze.', { exact: true })).toBeVisible();
});

test('keeps the language of each attempt in an old race', async ({ page }) => {
	const card = await seed(page);
	const round = await start(page);
	await review(page, card, round, { progressVersion: 0, direction: 'ru-pl', action: 'answer', answer: 'Dom' });
	sql(`UPDATE competitions SET direction = 'pl-ru' WHERE id = '${round.id}'`);
	await review(page, card, round, { progressVersion: 1, direction: 'pl-ru', action: 'answer', answer: 'Дом' });
	sql(`UPDATE competitions SET direction = NULL, version = version + 1 WHERE id = '${round.id}'`);
	await page.goto('/');
	await openRace(page, round.id);
	for (const summary of await page.locator('.race-history summary').all()) await summary.click();
	await expect(page.getByText(/^Odpowiedź:/).locator('[lang="pl"]')).toHaveText('Dom');
	await expect(page.getByText(/^Odpowiedź:/).locator('[lang="ru"]')).toHaveText('Дом');
});

test('retries a failed history read', async ({ page }) => {
	const card = await seed(page);
	const round = await start(page);
	await review(page, card, round, { progressVersion: 0, direction: 'ru-pl', action: 'answer', answer: 'Dom' });
	let reads = 0;
	await page.route(`**/api/competitions/${round.id}/attempts`, route => ++reads === 1 ? route.abort() : route.continue());
	await page.goto('/');
	await openRace(page, round.id);
	await expect(page.locator('.race-history').getByRole('alert')).toBeVisible();
	await page.locator('.race-history').getByRole('button', { name: 'Ponów', exact: true }).click();
	await expect(page.getByText(`${USER_A.displayName}: Dobrze`, { exact: true })).toBeVisible();
	await expect(page.locator('.race-history').getByRole('alert')).toHaveCount(0);
});
