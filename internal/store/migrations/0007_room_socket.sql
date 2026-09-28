-- The subtitle offset is room state, shared by everyone in the room, like the position.
ALTER TABLE rooms ADD COLUMN subtitle_offset_ms INTEGER NOT NULL DEFAULT 0;

-- Who has ever joined a room, for its "was here" list. Kept here so the list survives restarts.
CREATE TABLE room_visitors (
    room_id INTEGER NOT NULL REFERENCES rooms (id) ON DELETE CASCADE,
    user_id INTEGER NOT NULL REFERENCES users (id),
    PRIMARY KEY (room_id, user_id)
) STRICT, WITHOUT ROWID;
