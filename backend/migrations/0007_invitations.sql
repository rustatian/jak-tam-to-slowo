-- token_hash is hex text, because Aurora DSQL does not index the type bytea.
-- https://docs.aws.amazon.com/aurora-dsql/latest/userguide/working-with-postgresql-compatibility-supported-data-types.html
CREATE TABLE invitations (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	group_id uuid NOT NULL REFERENCES groups (id) ON DELETE CASCADE,
	email text NOT NULL,
	token_hash text NOT NULL UNIQUE,
	invited_by text REFERENCES users (id) ON DELETE SET NULL,
	created_at timestamptz NOT NULL DEFAULT now(),
	expires_at timestamptz NOT NULL,
	accepted_at timestamptz
);
