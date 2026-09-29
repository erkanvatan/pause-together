-- A room's chat, kept forever and deleted with its room.
CREATE TABLE messages (
    id          INTEGER PRIMARY KEY,
    room_id     INTEGER NOT NULL REFERENCES rooms (id) ON DELETE CASCADE,
    user_id     INTEGER NOT NULL REFERENCES users (id),
    -- '' once deleted. A deleted message keeps its row, so a reply to it can still say so.
    text        TEXT NOT NULL,
    reply_to    INTEGER REFERENCES messages (id), -- a message in the same room
    -- What was playing, and where, when it was sent. Video rows are never deleted, so no cascade.
    video_id    INTEGER NOT NULL REFERENCES videos (id),
    position_ms INTEGER NOT NULL,
    sent_at     INTEGER NOT NULL, -- wall clock, Unix milliseconds
    deleted     INTEGER NOT NULL DEFAULT 0
) STRICT;

CREATE INDEX messages_room ON messages (room_id, id);
-- Deleting a room deletes its messages, and each deleted row is checked for replies to it. Without
-- this index, every check reads the whole table.
CREATE INDEX messages_reply_to ON messages (reply_to);
