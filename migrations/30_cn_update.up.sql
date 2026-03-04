ALTER TABLE credit_notes
DROP CONSTRAINT credit_notes_type_check;

ALTER TABLE credit_notes
ADD CONSTRAINT credit_notes_type_check
CHECK (type IN ('return','adjustment','discount'));
