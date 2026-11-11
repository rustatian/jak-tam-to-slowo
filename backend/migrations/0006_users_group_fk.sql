-- Aurora DSQL adds a foreign key to an existing table only with NOT VALID. New and changed rows are still checked.
-- https://docs.aws.amazon.com/aurora-dsql/latest/userguide/alter-table-syntax-support.html
ALTER TABLE users ADD CONSTRAINT users_group_id_fkey FOREIGN KEY (group_id) REFERENCES groups (id) NOT VALID;
