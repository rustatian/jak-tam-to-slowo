-- An invitation is deleted when a person accepts it, so no code reads or writes accepted_at.
ALTER TABLE invitations DROP COLUMN accepted_at;
