-- Automatic Box shutdowns. Only actual attempts are recorded (a handful per
-- week), never the routine evaluations that decide against one -- those live
-- in the monitor's in-memory ring buffer.
CREATE TABLE box_auto_off_events (
    id          BIGSERIAL PRIMARY KEY,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    outcome     TEXT        NOT NULL,
    detail      JSONB       NOT NULL DEFAULT '{}'
);

CREATE INDEX idx_box_auto_off_events_occurred_at ON box_auto_off_events (occurred_at DESC);
