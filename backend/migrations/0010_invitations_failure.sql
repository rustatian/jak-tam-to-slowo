-- failure is bounce or complaint after the email service reported that for the invitation email
ALTER TABLE invitations ADD COLUMN failure text;
