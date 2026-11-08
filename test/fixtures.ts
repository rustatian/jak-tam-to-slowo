import { createHash } from 'node:crypto';
import type { GrammarContent } from '../shared/types.ts';

export const USER_A = {
	id: '11111111-1111-4111-8111-111111111111',
	username: 'anna-test',
	displayName: 'Анна',
	password: 'test-anna-password-000001',
};
export const USER_B = {
	id: '22222222-2222-4222-8222-222222222222',
	username: 'boris-test',
	displayName: 'Борис',
	password: 'test-boris-password-00001',
};
export const testAccountSecret = JSON.stringify([USER_A, USER_B].map(({ password, ...user }) => ({
	...user, passwordHash: createHash('sha256').update(password).digest('hex'),
})));

export const testBindings = {
	ACCOUNTS: testAccountSecret,
	ANTHROPIC_API_KEY: 'test-anthropic-key',
	OPENAI_TRANSCRIPTION_API_KEY: 'test-transcription-key',
};

export const CARD_A = {
	id: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa',
	pl: 'Słowo', ru: 'Слово', meaningRu: 'Единица речи',
	examplePl: 'To nowe słowo.', exampleRu: 'Это новое слово.',
	alternativesPl: ['Wyraz'], alternativesRu: ['Словечко'],
};
export const CARD_B = {
	id: 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb',
	pl: 'Dom', ru: 'Дом', meaningRu: 'Жилое здание',
	examplePl: 'To nasz dom.', exampleRu: 'Это наш дом.',
	alternativesPl: [], alternativesRu: ['Жилище'],
};

export const WORD_DRAFT = {
	pl: 'Słowo', ru: 'Слово', meaningRu: 'Единица речи',
	examplePl: 'Nie znam tego słowa.', exampleRu: 'Я не знаю этого слова.',
	alternativesPl: ['Wyraz'], alternativesRu: [],
};
export const ONION_DRAFT = {
	pl: 'Cebula', ru: 'Лук', meaningRu: 'Овощ',
	examplePl: 'Kroję cebulę.', exampleRu: 'Я режу лук.',
	alternativesPl: [], alternativesRu: [],
};

export const GRAMMAR_DRAFT = {
	pl: 'pies', examplePl: 'Na podwórku biegają dwa ___.', alternativesPl: [],
	grammar: {
		used: 'dwa piesy', right: 'dwa psy', answer: 'psy',
		facts: ['rzeczownik', 'rodzaj męski żywotny'],
		slot: ['Mianownik', 'liczba mnoga', 'niemęskoosobowy', 'końcówka -y'],
		mistake: 'e ruchome',
		table: { title: 'Mianownik, liczba mnoga: końcówki według rodzaju', columns: ['Rodzaj', 'Końcówka', 'Przykład'], groups: [
			{ title: 'W liczbie mnogiej: męskoosobowy', rows: [{ names: ['męski osobowy'], endings: '-i, -y, -owie, -e', examples: ['student → studenci'], hit: false }] },
			{ title: 'W liczbie mnogiej: niemęskoosobowy', rows: [
				{ names: ['męski żywotny', 'męski nieżywotny', 'żeński'], endings: '-y, -i, -e', examples: ['pies → psy', 'dom → domy', 'kobieta → kobiety'], hit: true },
				{ names: ['nijaki'], endings: '-a', examples: ['okno → okna'], hit: false },
			] },
		] },
		explanation: 'W liczbie mnogiej „pies” traci „e”: psy. Tak samo: sen – sny, dzień – dni.',
	},
} satisfies GrammarContent;
export const GRAMMAR_CARD = { id: 'eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee', kind: 'grammar' as const, ...GRAMMAR_DRAFT };
