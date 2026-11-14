-- One row is one counted answer of one learner.
-- The app makes attempt_id, and a repeat of the request finds the row by the primary key.
-- card_id has no foreign key: the table grows with each answer, so it stays out of the cascade of a group delete,
-- because Aurora DSQL lets one transaction change 3,000 rows.
-- The row holds no text of the card.
-- https://docs.aws.amazon.com/aurora-dsql/latest/userguide/CHAP_quotas.html#SECTION_database-limits
-- https://docs.aws.amazon.com/aurora-dsql/latest/userguide/create-table-syntax-support.html#create-table-fk-actions
CREATE TABLE reviews (
	user_id text NOT NULL REFERENCES users (id),
	attempt_id uuid NOT NULL,
	card_id uuid NOT NULL,
	grade text NOT NULL CHECK (grade IN ('correct', 'almost', 'again')),
	PRIMARY KEY (user_id, attempt_id)
);
