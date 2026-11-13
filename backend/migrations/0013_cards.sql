-- The primary key starts with group_id, so a read of the cards of one group is a range of the primary key.
-- UNIQUE (id) is the target of later foreign keys to a card.
-- shortcut: Aurora DSQL lets one transaction change 3,000 rows, and cascaded rows count, so a group with more than
-- 3,000 cards cannot be deleted in one transaction. Delete the cards in parts before a group can have that many.
-- https://docs.aws.amazon.com/aurora-dsql/latest/userguide/CHAP_quotas.html#SECTION_database-limits
-- https://docs.aws.amazon.com/aurora-dsql/latest/userguide/create-table-syntax-support.html#create-table-fk-actions
CREATE TABLE cards (
	group_id uuid NOT NULL REFERENCES groups (id) ON DELETE CASCADE,
	id uuid NOT NULL DEFAULT gen_random_uuid(),
	kind text NOT NULL DEFAULT 'translation' CHECK (kind IN ('translation', 'grammar')),
	pl text NOT NULL,
	ru text NOT NULL,
	example_pl text NOT NULL,
	example_ru text NOT NULL,
	meaning_ru text NOT NULL,
	alternatives_pl jsonb NOT NULL DEFAULT '[]',
	alternatives_ru jsonb NOT NULL DEFAULT '[]',
	archived boolean NOT NULL DEFAULT false,
	created_at timestamptz NOT NULL DEFAULT now(),
	PRIMARY KEY (group_id, id),
	UNIQUE (id)
);
