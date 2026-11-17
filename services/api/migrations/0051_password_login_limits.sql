-- +goose Up
-- Username-scoped keyed digests also cover unknown usernames with identical behavior.
CREATE TABLE c_auth.password_login_limits (
 subject_digest bytea PRIMARY KEY CHECK (octet_length(subject_digest)=32),
 failures smallint NOT NULL DEFAULT 0 CHECK (failures BETWEEN 0 AND 5),
 locked_until timestamptz,
 CHECK ((failures<5 AND locked_until IS NULL) OR (failures=5 AND locked_until IS NOT NULL))
);

-- +goose Down
DROP TABLE c_auth.password_login_limits;
