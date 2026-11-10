CREATE TABLE users (
	id text PRIMARY KEY,
	email text NOT NULL,
	display_name text NOT NULL DEFAULT '',
	direction text NOT NULL DEFAULT 'ru-pl' CHECK (direction IN ('ru-pl', 'pl-ru')),
	created_at timestamptz NOT NULL DEFAULT now()
);
