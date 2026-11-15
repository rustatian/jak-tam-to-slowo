-- NULL for a translation card. The server writes the object of a grammar card.
ALTER TABLE cards ADD COLUMN grammar jsonb;
