-- +goose Up
CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email text NOT NULL,
    password_hash text NOT NULL,
    profile_version bigint NOT NULL DEFAULT 1 CHECK (profile_version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT users_email_unique UNIQUE (email),
    CONSTRAINT users_email_normalized CHECK (
        email = lower(btrim(email)) AND char_length(email) BETWEEN 3 AND 254
        AND email ~ '^[^[:space:]@]+@[^[:space:]@]+$'
    ),
    CONSTRAINT users_password_hash_bcrypt CHECK (
        char_length(password_hash) = 60 AND password_hash ~ '^\$2[aby]\$[0-9]{2}\$[./A-Za-z0-9]{53}$'
    )
);

-- Contact email is independent of the login identity; all fields are optional.
CREATE TABLE user_profiles (
    user_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    display_name text CHECK (char_length(display_name) BETWEEN 1 AND 100),
    contact_email text CHECK (char_length(contact_email) BETWEEN 3 AND 254 AND contact_email ~ '^[^[:space:]@]+@[^[:space:]@]+$'),
    phone text CHECK (char_length(phone) BETWEEN 1 AND 32),
    city text CHECK (char_length(city) BETWEEN 1 AND 100),
    country_code text CHECK (country_code ~ '^[A-Z]{2}$'),
    summary text CHECK (char_length(summary) BETWEEN 1 AND 2000),
    github_url text CHECK (char_length(github_url) <= 2048 AND github_url ~ '^https://[^[:space:]]+$'),
    linkedin_url text CHECK (char_length(linkedin_url) <= 2048 AND linkedin_url ~ '^https://[^[:space:]]+$'),
    website_url text CHECK (char_length(website_url) <= 2048 AND website_url ~ '^https://[^[:space:]]+$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

COMMENT ON COLUMN users.profile_version IS
    'Profile writes must lock users and compare the expected version; the service increments once per successful mutation in that same transaction.';
COMMENT ON COLUMN user_profiles.github_url IS
    'Database checks scheme/length only; HTTP layer must parse and validate the URL.';

-- +goose Down
DROP TABLE user_profiles;
DROP TABLE users;
