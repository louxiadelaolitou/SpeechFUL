-- +goose Up
-- Short-lived proof state only; independent C/V stores, no account/email data.
CREATE TABLE v_auth.captcha_challenges (
 id bytea PRIMARY KEY CHECK (octet_length(id)=32),
 binding bytea NOT NULL CHECK (octet_length(binding)=32),
 answer_x integer NOT NULL CHECK (answer_x BETWEEN 0 AND 320),
 expires_at timestamptz NOT NULL
);
CREATE INDEX captcha_challenges_expiry ON v_auth.captcha_challenges(expires_at);
CREATE TABLE v_auth.captcha_proofs (
 id bytea PRIMARY KEY CHECK (octet_length(id)=32),
 binding bytea NOT NULL CHECK (octet_length(binding)=32),
 expires_at timestamptz NOT NULL
);
CREATE INDEX captcha_proofs_expiry ON v_auth.captcha_proofs(expires_at);

-- +goose Down
DROP TABLE v_auth.captcha_proofs;
DROP TABLE v_auth.captcha_challenges;
