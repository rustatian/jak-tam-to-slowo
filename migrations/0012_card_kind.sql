ALTER TABLE cards ADD COLUMN kind TEXT NOT NULL DEFAULT 'translation' CHECK (kind IN ('translation', 'grammar'));
ALTER TABLE cards ADD COLUMN grammar TEXT;
