-- A removed library keeps its row, since videos.library_id points at it. Adding the same folder again
-- brings the row back, so its videos keep their ids.
ALTER TABLE libraries ADD COLUMN removed INTEGER NOT NULL DEFAULT 0;
