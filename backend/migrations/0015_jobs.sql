-- One row is one background job of one user.
-- user_id is first in the primary key, because each reader has the user and the cascade from users finds the rows
-- by user_id.
-- kind takes any text, so a later job kind needs no migration.
-- result and error are NULL until the job ends.
-- shortcut: the delete of a user deletes the job rows of the user in the same transaction, and Aurora DSQL lets one
-- transaction change 3,000 rows. Keep the job rows of one user below that count.
-- https://docs.aws.amazon.com/aurora-dsql/latest/userguide/CHAP_quotas.html#SECTION_database-limits
-- https://docs.aws.amazon.com/aurora-dsql/latest/userguide/create-table-syntax-support.html#create-table-fk-actions
CREATE TABLE jobs (
	user_id text NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	id uuid NOT NULL DEFAULT gen_random_uuid(),
	kind text NOT NULL,
	status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'done', 'failed')),
	input jsonb NOT NULL,
	result jsonb,
	error text,
	created_at timestamptz NOT NULL DEFAULT now(),
	PRIMARY KEY (user_id, id)
);
