ALTER TABLE competitions ADD COLUMN direction TEXT CHECK (direction IN ('ru-pl', 'pl-ru'));
UPDATE competitions SET direction = (SELECT direction FROM users WHERE users.id = competitions.created_by) WHERE status IN ('waiting', 'active');
