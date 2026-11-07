import type { Direction } from '../shared/types';

export const directionNames: Record<Direction, string> = { 'ru-pl': 'Rosyjski → Polski', 'pl-ru': 'Polski → Rosyjski' };
export const directionShort: Record<Direction, string> = { 'ru-pl': 'RU → PL', 'pl-ru': 'PL → RU' };
export const statusNames: Record<'waiting' | 'active' | 'finished', string> = { waiting: 'Czeka', active: 'Trwa', finished: 'Zakończony' };
