-- One row is the progress of one learner for one card, for both directions.
-- card_id is first in the primary key, because the cascade from cards finds the rows by card_id.
-- A NULL time means that the learner has no such answer for the card.
-- shortcut: the delete of a group deletes its cards and their progress rows in one transaction, and Aurora DSQL
-- lets one transaction change 3,000 rows. Delete in parts before the cards and the progress rows of one group can
-- pass that count.
-- https://docs.aws.amazon.com/aurora-dsql/latest/userguide/CHAP_quotas.html#SECTION_database-limits
-- https://docs.aws.amazon.com/aurora-dsql/latest/userguide/create-table-syntax-support.html#create-table-fk-actions
CREATE TABLE card_progress (
	user_id text NOT NULL REFERENCES users (id),
	card_id uuid NOT NULL REFERENCES cards (id) ON DELETE CASCADE,
	correct integer NOT NULL,
	incorrect integer NOT NULL,
	last_correct_at timestamptz,
	last_incorrect_at timestamptz,
	version integer NOT NULL,
	PRIMARY KEY (card_id, user_id)
);
