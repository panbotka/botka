-- Task tags: colored labels for categorizing tasks (bug, feature, refactor, ...).
-- A separate table from `tags` (which is used for chat threads) so the two
-- domains can evolve independently.

CREATE TABLE task_tags (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(50) NOT NULL,
    color VARCHAR(7) NOT NULL DEFAULT '#6B7280',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Case-insensitive uniqueness on name.
CREATE UNIQUE INDEX idx_task_tags_name_lower ON task_tags (LOWER(name));

CREATE TABLE task_tag_assignments (
    task_id UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    tag_id BIGINT NOT NULL REFERENCES task_tags(id) ON DELETE CASCADE,
    PRIMARY KEY (task_id, tag_id)
);

CREATE INDEX idx_task_tag_assignments_tag_id ON task_tag_assignments(tag_id);

-- Seed a small palette of default tags so the UI is useful out of the box.
INSERT INTO task_tags (name, color) VALUES
    ('bug',       '#EF4444'),
    ('feature',   '#3B82F6'),
    ('refactor',  '#A855F7'),
    ('chore',     '#6B7280'),
    ('infra',     '#14B8A6'),
    ('docs',      '#F59E0B');
