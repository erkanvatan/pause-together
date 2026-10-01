-- When a room was last used, on the wall clock (Unix milliseconds): made, switched, unarchived, or moved
-- to a new position.
-- The homepage lists rooms by it. Rooms made before this start at their last chat message, or 0.
ALTER TABLE rooms ADD COLUMN used_at INTEGER NOT NULL DEFAULT 0;
UPDATE rooms SET used_at = COALESCE((SELECT MAX(sent_at) FROM messages WHERE room_id = rooms.id), 0);
