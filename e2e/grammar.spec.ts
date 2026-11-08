import { expect, test } from '@playwright/test';
import { GRAMMAR_DRAFT, USER_A } from '../test/fixtures';
import { allCards } from './cards';

let createdIds: string[];
test.beforeEach(async ({ page, baseURL }) => {
	createdIds = [];
	const headers = { Origin: baseURL!, 'CF-Connecting-IP': `grammar-${crypto.randomUUID()}` };
	await page.setExtraHTTPHeaders(headers);
	expect((await page.request.post('/api/session', { headers, data: { username: USER_A.username, password: USER_A.password } })).status()).toBe(200);
	await page.goto('/');
});

test.afterEach(async ({ page, baseURL }) => {
	const headers = { Origin: baseURL! };
	const list = await allCards(page.request);
	for (const card of list.filter(card => createdIds.includes(card.id))) {
		expect((await page.request.patch(`/api/cards/${card.id}`, { headers, data: { version: card.version, archived: true } })).status()).toBe(200);
	}
});

test('adds a grammar card and shows the struck word, the chips and the marked table row', async ({ page }) => {
	await page.getByRole('button', { name: 'Dodaj', exact: true }).click();
	await page.getByRole('group', { name: 'Rodzaj fiszki' }).getByRole('button', { name: 'Gramatyka', exact: true }).click();
	await page.getByLabel('Słowo po polsku').fill('pies');
	await page.getByLabel('Jak tego użyłeś').fill('dwa piesy');
	const generated = page.waitForResponse(response => response.url().endsWith('/api/cards/draft'));
	await page.getByRole('button', { name: 'Sprawdź gramatykę', exact: true }).click();
	expect((await generated).request().postDataJSON()).toEqual({ kind: 'grammar', word: 'pies', used: 'dwa piesy' });
	await expect(page.getByRole('heading', { name: 'Sprawdź fiszkę', exact: true })).toBeVisible();
	const result = page.locator('.result');
	await expect(result.locator('.form-line').first()).toHaveText('dwa piesy');
	await expect(result.locator('.form-line').first().locator('s.wrong')).toHaveText('piesy');
	await expect(result.locator('.form-line').nth(1).locator('b.right')).toHaveText('psy');
	await expect(result.locator('.labels .lab')).toHaveText(['rzeczownik', 'rodzaj męski żywotny', 'Mianownik', 'liczba mnoga', 'niemęskoosobowy', 'końcówka -y', 'błąd: e ruchome']);
	const table = page.getByRole('table', { name: 'Mianownik, liczba mnoga: końcówki według rodzaju' });
	await expect(table.locator('tr.group th')).toHaveText(['W liczbie mnogiej: męskoosobowy', 'W liczbie mnogiej: niemęskoosobowy']);
	await expect(table.locator('tr.hit th b.right')).toHaveText('męski żywotny');
	await expect(table.locator('tr.hit td').first().locator('b.right')).toHaveText('-y');
	await expect(table.locator('tr.hit td').nth(1).locator('.kindrow').first()).toHaveText('pies → piesy psy');
	await expect(table.locator('tr.hit td').nth(1).locator('s.wrong')).toHaveText('piesy');
	await expect(page.getByLabel('Poprawna forma')).toHaveValue('psy');
	await expect(page.getByLabel('Zdanie z luką')).toHaveValue('Na podwórku biegają dwa ___.');
	const saved = page.waitForResponse(response => response.url().endsWith('/api/cards') && response.request().method() === 'POST');
	await page.getByRole('button', { name: 'Zapisz', exact: true }).click();
	const body = (await saved).request().postDataJSON();
	createdIds.push(body.id);
	expect(body).toEqual({ id: body.id, kind: 'grammar', pl: 'pies', examplePl: 'Na podwórku biegają dwa ___.', alternativesPl: [], grammar: GRAMMAR_DRAFT.grammar });
	expect((await saved).status()).toBe(201);
	await expect(page.getByText('Fiszka zapisana.', { exact: true })).toBeVisible();
});

test('marks the whole phrase when the word counts differ', async ({ page }) => {
	// The fake provider echoes GRAMMAR_DRAFT, so seed the screen through session storage instead.
	const card = structuredClone(GRAMMAR_DRAFT);
	card.grammar.used = 'idę sklep';
	card.grammar.right = 'idę do sklepu';
	await page.evaluate(({ key, draft }) => sessionStorage.setItem(key, JSON.stringify(draft)), { key: `grammar-draft:${USER_A.id}`, draft: { id: crypto.randomUUID(), word: 'sklep', used: 'idę sklep', card } });
	await page.evaluate(key => sessionStorage.setItem(key, JSON.stringify('grammar')), `add-mode:${USER_A.id}`);
	await page.reload();
	await page.getByRole('button', { name: 'Dodaj', exact: true }).click();
	const result = page.locator('.result');
	await expect(result.locator('.form-line').first().locator('s.wrong')).toHaveText(['idę', 'sklep']);
	await expect(result.locator('.form-line').nth(1).locator('b.right')).toHaveText(['idę', 'do', 'sklepu']);
	await expect(page.getByRole('table', { name: 'Mianownik, liczba mnoga: końcówki według rodzaju' }).locator('tr.hit td').nth(1).locator('s.wrong')).toHaveText('idę sklep');
});

test('restores the grammar draft and the mode after a reload', async ({ page }) => {
	await page.getByRole('button', { name: 'Dodaj', exact: true }).click();
	await page.getByRole('group', { name: 'Rodzaj fiszki' }).getByRole('button', { name: 'Gramatyka', exact: true }).click();
	await page.getByLabel('Słowo po polsku').fill('pies');
	await page.getByLabel('Jak tego użyłeś').fill('dwa piesy');
	await page.getByRole('button', { name: 'Sprawdź gramatykę', exact: true }).click();
	await expect(page.getByLabel('Poprawna forma')).toHaveValue('psy');
	await page.reload();
	await page.getByRole('button', { name: 'Dodaj', exact: true }).click();
	await expect(page.getByRole('button', { name: 'Gramatyka', exact: true })).toHaveAttribute('aria-pressed', 'true');
	await expect(page.getByLabel('Słowo po polsku')).toHaveValue('pies');
	await expect(page.getByLabel('Jak tego użyłeś')).toHaveValue('dwa piesy');
	await expect(page.getByLabel('Poprawna forma')).toHaveValue('psy');
	await page.getByRole('group', { name: 'Rodzaj fiszki' }).getByRole('button', { name: 'Tłumaczenie', exact: true }).click();
	await expect(page.getByLabel('Słowo lub wyrażenie')).toBeVisible();
	await page.getByRole('group', { name: 'Rodzaj fiszki' }).getByRole('button', { name: 'Gramatyka', exact: true }).click();
	await expect(page.getByLabel('Poprawna forma')).toHaveValue('psy');
});
