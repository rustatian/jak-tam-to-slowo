-- An invitation without a group is a new group invitation of the operator: its acceptance creates the group.
ALTER TABLE invitations ALTER COLUMN group_id DROP NOT NULL;
