-- +goose Up
-- Remove journals whose conversation was already deleted. Suggestions and
-- titles deliberately have no messages row, but still belong to a session.
DELETE FROM output_review_private
WHERE NOT EXISTS (SELECT 1 FROM sessions WHERE id = session_id)
   OR (kind IN ('answer', 'summary') AND NOT EXISTS (
       SELECT 1 FROM messages WHERE id = message_id AND messages.session_id = output_review_private.session_id
   ) AND NOT EXISTS (
       SELECT 1 FROM output_review_private AS auxiliary
       WHERE auxiliary.message_id = output_review_private.message_id
         AND auxiliary.session_id = output_review_private.session_id
         AND auxiliary.kind NOT IN ('answer', 'summary')
   ));
DELETE FROM output_review_public
WHERE NOT EXISTS (SELECT 1 FROM sessions WHERE id = session_id)
   OR (NOT EXISTS (SELECT 1 FROM messages WHERE id = message_id AND messages.session_id = output_review_public.session_id)
       AND NOT EXISTS (SELECT 1 FROM output_review_private WHERE message_id = output_review_public.message_id AND session_id = output_review_public.session_id));
DELETE FROM output_review_events
WHERE NOT EXISTS (SELECT 1 FROM sessions WHERE id = session_id)
   OR (NOT EXISTS (SELECT 1 FROM messages WHERE id = message_id AND messages.session_id = output_review_events.session_id)
       AND NOT EXISTS (SELECT 1 FROM output_review_private WHERE message_id = output_review_events.message_id AND session_id = output_review_events.session_id));
DELETE FROM output_review_corrections
WHERE NOT EXISTS (SELECT 1 FROM messages WHERE id = message_id)
  AND NOT EXISTS (SELECT 1 FROM output_review_private WHERE message_id = output_review_corrections.message_id);
DELETE FROM output_review_attempts
WHERE NOT EXISTS (SELECT 1 FROM messages WHERE id = message_id)
  AND NOT EXISTS (SELECT 1 FROM output_review_private WHERE message_id = output_review_attempts.message_id);

-- +goose StatementBegin
CREATE TRIGGER output_review_message_delete AFTER DELETE ON messages BEGIN
    DELETE FROM output_review_corrections WHERE message_id = OLD.id;
    DELETE FROM output_review_attempts WHERE message_id = OLD.id;
    DELETE FROM output_review_events WHERE message_id = OLD.id;
    DELETE FROM output_review_public WHERE message_id = OLD.id;
    DELETE FROM output_review_private WHERE message_id = OLD.id;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER output_review_session_delete AFTER DELETE ON sessions BEGIN
    DELETE FROM output_review_corrections WHERE message_id IN (
        SELECT message_id FROM output_review_private WHERE session_id = OLD.id
        UNION SELECT message_id FROM output_review_public WHERE session_id = OLD.id
    );
    DELETE FROM output_review_attempts WHERE message_id IN (
        SELECT message_id FROM output_review_private WHERE session_id = OLD.id
        UNION SELECT message_id FROM output_review_public WHERE session_id = OLD.id
    );
    DELETE FROM output_review_events WHERE session_id = OLD.id;
    DELETE FROM output_review_public WHERE session_id = OLD.id;
    DELETE FROM output_review_private WHERE session_id = OLD.id;
END;
-- +goose StatementEnd

-- Reject callbacks that finish after the owning message/session was deleted.
-- +goose StatementBegin
CREATE TRIGGER output_review_private_owner BEFORE INSERT ON output_review_private
WHEN NOT EXISTS (SELECT 1 FROM sessions WHERE id = NEW.session_id)
  OR (NEW.kind IN ('answer', 'summary') AND NOT EXISTS (
      SELECT 1 FROM messages WHERE id = NEW.message_id AND session_id = NEW.session_id
  )) BEGIN
    SELECT RAISE(ABORT, 'output review owner no longer exists');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER output_review_public_owner BEFORE INSERT ON output_review_public
WHEN NOT EXISTS (SELECT 1 FROM sessions WHERE id = NEW.session_id)
  OR (NOT EXISTS (SELECT 1 FROM messages WHERE id = NEW.message_id AND session_id = NEW.session_id)
      AND NOT EXISTS (SELECT 1 FROM output_review_private WHERE message_id = NEW.message_id AND session_id = NEW.session_id)) BEGIN
    SELECT RAISE(ABORT, 'output review owner no longer exists');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER output_review_event_owner BEFORE INSERT ON output_review_events
WHEN NOT EXISTS (SELECT 1 FROM sessions WHERE id = NEW.session_id)
  OR (NOT EXISTS (SELECT 1 FROM messages WHERE id = NEW.message_id AND session_id = NEW.session_id)
      AND NOT EXISTS (SELECT 1 FROM output_review_private WHERE message_id = NEW.message_id AND session_id = NEW.session_id)) BEGIN
    SELECT RAISE(ABORT, 'output review owner no longer exists');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER output_review_correction_owner BEFORE INSERT ON output_review_corrections
WHEN NOT EXISTS (SELECT 1 FROM messages WHERE id = NEW.message_id)
 AND NOT EXISTS (SELECT 1 FROM output_review_private WHERE message_id = NEW.message_id) BEGIN
    SELECT RAISE(ABORT, 'output review owner no longer exists');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER output_review_attempt_owner BEFORE INSERT ON output_review_attempts
WHEN NOT EXISTS (SELECT 1 FROM messages WHERE id = NEW.message_id)
 AND NOT EXISTS (SELECT 1 FROM output_review_private WHERE message_id = NEW.message_id) BEGIN
    SELECT RAISE(ABORT, 'output review owner no longer exists');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER output_review_attempt_owner;
DROP TRIGGER output_review_correction_owner;
DROP TRIGGER output_review_event_owner;
DROP TRIGGER output_review_public_owner;
DROP TRIGGER output_review_private_owner;
DROP TRIGGER output_review_session_delete;
DROP TRIGGER output_review_message_delete;
