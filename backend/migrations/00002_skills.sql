-- +goose Up
CREATE TABLE skills (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL CHECK (char_length(btrim(name)) BETWEEN 1 AND 100),
    slug text NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX skills_normalized_name_unique ON skills (lower(btrim(name)));

-- Canonical spellings are also aliases. All catalogue search uses this table.
CREATE TABLE skill_aliases (
    alias text PRIMARY KEY CHECK (alias = lower(btrim(alias)) AND char_length(alias) BETWEEN 1 AND 100),
    skill_id uuid NOT NULL REFERENCES skills(id) ON DELETE CASCADE
);
CREATE INDEX skill_aliases_skill_id ON skill_aliases(skill_id);

CREATE TABLE user_skills (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    skill_id uuid NOT NULL REFERENCES skills(id) ON DELETE RESTRICT,
    proficiency text NOT NULL CHECK (proficiency IN ('new', 'learning', 'working', 'proficient')),
    notes text NOT NULL DEFAULT '' CHECK (char_length(notes) <= 1000),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, skill_id)
);
COMMENT ON TABLE user_skills IS 'Absence means not_assessed, never a missing ability.';

-- +goose Down
DROP TABLE user_skills;
DROP TABLE skill_aliases;
DROP TABLE skills;
