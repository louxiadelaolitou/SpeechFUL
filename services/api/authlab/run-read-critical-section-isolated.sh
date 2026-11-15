#!/bin/sh
# REQ-001 service-method measurement. Creates only its own private SQL fixture.
# Requires preinstalled tools and real validator fixtures; never accepts a DSN.
set -eu
umask 077
[ "$(id -u)" -ne 0 ] || { echo 'Run as a non-root task user' >&2; exit 2; }
read_source=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
case "$read_source" in /mnt/*) echo 'Run source from the WSL Linux filesystem' >&2; exit 2 ;; esac
for read_tool in go initdb pg_ctl psql; do
    command -v "$read_tool" >/dev/null 2>&1 || { echo "Missing tool: $read_tool" >&2; exit 2; }
done
: "${REQ001_BASELINE_OUTPUT:?Absolute artifact file outside source required}"
: "${REQ001_BASELINE_SOURCE:?Exact tested source commit or baseline plus diff required}"
: "${POST_MEDIA_PYTHON:?Real image validator Python required}"
: "${POST_MEDIA_FIXTURES:?Real generated image fixtures required}"
: "${COMMENT_SNAPSHOT_REDIS_SERVER:?Owned Redis executable required}"
[ ! -e "$REQ001_BASELINE_OUTPUT" ] || { echo 'Refusing to overwrite an existing baseline artifact' >&2; exit 2; }
read_port=${PGPORT:-55531}
case "$read_port" in ''|*[!0-9]*|0*) echo 'Invalid fixture port' >&2; exit 2 ;; esac
[ "${#read_port}" -le 5 ] && [ "$read_port" -ge 1024 ] && [ "$read_port" -le 65535 ] || exit 2
PGPORT=$read_port
export PGPORT
read_dir=$(mktemp -d /tmp/hnuhole-req001-read.XXXXXX)
case "$read_dir" in *[!A-Za-z0-9_./-]*) echo 'Unsupported fixture path' >&2; exit 2 ;; esac
mkdir "$read_dir/socket"
POST_MEDIA_VALIDATOR="$read_dir/validate_image.py"
cp "$read_source/media/validate_image.py" "$POST_MEDIA_VALIDATOR"
chmod 600 "$POST_MEDIA_VALIDATOR"
export POST_MEDIA_VALIDATOR
read_cleanup() {
    read_status=$?
    pg_ctl -D "$read_dir/data" -m fast -w stop >/dev/null 2>&1 || true
    rm -rf "$read_dir"
    exit "$read_status"
}
trap read_cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM HUP
initdb -D "$read_dir/data" -U req001_read_admin -A trust --no-locale -E UTF8 >"$read_dir/init.log" 2>&1
pg_ctl -D "$read_dir/data" -l "$read_dir/postgres.log" -w start \
    -o "-p $read_port -c listen_addresses='' -c unix_socket_directories='$read_dir/socket' -c unix_socket_permissions=0700 -c shared_memory_type=mmap -c dynamic_shared_memory_type=mmap -c shared_buffers=16MB -c max_connections=40 -c log_statement=none -c log_min_messages=panic -c log_min_error_statement=panic -c log_error_verbosity=terse -c log_parameter_max_length=0 -c log_parameter_max_length_on_error=0 -c log_min_duration_statement=-1 -c log_min_duration_sample=-1 -c log_transaction_sample_rate=0 -c log_duration=off" >/dev/null
psql -X -v ON_ERROR_STOP=1 -h "$read_dir/socket" -U req001_read_admin -d postgres >/dev/null <<'SQL'
CREATE ROLE hnuhole_c LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE;
CREATE ROLE hnuhole_v LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE;
CREATE ROLE hnuhole_c_runtime LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE;
CREATE ROLE hnuhole_c_recovery NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE;
CREATE ROLE hnuhole_business NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE;
CREATE DATABASE hnuhole_c OWNER hnuhole_c;
CREATE DATABASE hnuhole_v OWNER hnuhole_v;
REVOKE CONNECT ON DATABASE hnuhole_c,hnuhole_v FROM PUBLIC;
GRANT CONNECT ON DATABASE hnuhole_c TO hnuhole_c,hnuhole_c_runtime;
GRANT CONNECT ON DATABASE hnuhole_v TO hnuhole_v;
SQL
AUTHLAB_RUNTIME_TAG="hnuhole_authlab_req001_read_$(date -u +%Y%m%d%H%M%S)"
AUTHLAB_C_DSN="postgresql://hnuhole_c@/hnuhole_c?host=$read_dir/socket&application_name=$AUTHLAB_RUNTIME_TAG&sslmode=disable"
AUTHLAB_V_DSN="postgresql://hnuhole_v@/hnuhole_v?host=$read_dir/socket&application_name=$AUTHLAB_RUNTIME_TAG&sslmode=disable"
AUTHLAB_POSTS_RUNTIME_DSN="postgresql://hnuhole_c_runtime@/hnuhole_c?host=$read_dir/socket&application_name=$AUTHLAB_RUNTIME_TAG&sslmode=disable"
AUTHLAB_ALLOW_SCHEMA_RESET=1
REQ001_BASELINE=1
export AUTHLAB_RUNTIME_TAG AUTHLAB_C_DSN AUTHLAB_V_DSN AUTHLAB_POSTS_RUNTIME_DSN AUTHLAB_ALLOW_SCHEMA_RESET REQ001_BASELINE
cd "$read_source"
go test -mod=readonly -count=1 -timeout=10m -p 1 ./internal/authprivacy -run '^TestReadCriticalSectionBaselinePostgres$' -v
