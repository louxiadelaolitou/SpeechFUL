#!/bin/sh
# Operator-owned projection only. Each invocation of the SQL batch commits
# independently; cancellation/error preserves the last committed cursor.
set -eu
umask 077
: "${POST_SEARCH_BACKFILL_DSN:?set the migration-owner DSN explicitly}"
batch=${1:-1000}
case "$batch" in ''|*[!0-9]*|0*) echo 'batch must be 1..10000' >&2; exit 2 ;; esac
[ "${#batch}" -le 5 ] && [ "$batch" -le 10000 ] || exit 2
export PGOPTIONS="${PGOPTIONS:-} -c statement_timeout=60s -c lock_timeout=5s"
search_psql() { psql -X -q -v ON_ERROR_STOP=1 --dbname="$POST_SEARCH_BACKFILL_DSN" "$@"; }
owner=$(search_psql -At -c "SELECT current_user=pg_get_userbyid(relowner) FROM pg_class WHERE oid=to_regclass('c_posts.post_search_documents')")
[ "$owner" = t ] || { echo 'migration owner and installed 0049 are required' >&2; exit 2; }
retries=0
while :; do
    processed=$(search_psql -At -c "SELECT c_posts.backfill_post_search_documents($batch)")
    if [ "$processed" = -1 ]; then
        retries=$((retries+1))
        [ "$retries" -le 10 ] || { echo 'source row still busy; committed cursor retained, retry later' >&2; exit 75; }
        sleep 1
        continue
    fi
    retries=0
    case "$processed" in ''|*[!0-9]*) echo 'unexpected backfill result' >&2; exit 1 ;; esac
    [ "$processed" -ne 0 ] || break
    printf 'committed %s source posts\n' "$processed"
done
search_psql -c 'SELECT c_posts.validate_post_search_documents()'
search_psql -c 'VACUUM (ANALYZE) c_posts.post_search_documents'
printf 'projection reconciled; searchDocuments may now be enabled\n'
