package authprivacy

import (
	"context"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/readtiming"
)

// A combined field is only an index candidate: exact predicates still inspect
// each original field/tag before relevance, grouping, LIMIT and cursors. In
// particular, a match spanning two tags must never enter the ranked set.
var documentSearchCandidatesSQL = strings.NewReplacer(
	`FROM c_posts.posts p
	JOIN c_posts.publication_tasks t ON t.post_id = p.post_id
	JOIN c_posts.attempt_contents x ON x.task_id = t.task_id AND x.version = p.published_attempt_version`,
	`FROM c_posts.post_search_documents d JOIN c_posts.posts p ON p.post_id=d.post_id`,
	`lower(COALESCE(to_jsonb(p)->>'edited_title',x.title) COLLATE pg_catalog."und-x-icu")`, `d.title_norm`,
	`lower(COALESCE(to_jsonb(p)->>'edited_body',x.body) COLLATE pg_catalog."und-x-icu")`, `d.body_norm`,
	`lower($2::text COLLATE pg_catalog."und-x-icu")`, `lower($2::text COLLATE pg_catalog."und-x-icu") COLLATE "C"`,
	`AND x.erased_at IS NULL`, `AND d.source_version=p.published_attempt_version AND d.content_revision=COALESCE(p.edit_revision,p.post_id)`,
	`jsonb_array_elements_text(COALESCE(to_jsonb(p)->'tags','[]'::jsonb))`, `unnest(d.tags_norm)`,
	`lower(tag COLLATE pg_catalog."und-x-icu")`, `tag`,
).Replace(searchCandidatesSQL)

// pg_trgm classifies characters using the database LC_CTYPE; C-locale databases
// extract NO Chinese trigrams even with an ICU index collation. Hex UTF-8 gives
// it ASCII trigrams while preserving every literal substring. False positives
// (byte/tag boundaries) are removed by the original exact predicates. Encoding
// also escapes %, _ and backslashes structurally; none become LIKE operators.
var indexedDocumentSearchCandidatesSQL = strings.Replace(documentSearchCandidatesSQL,
	`AND p.publication_ordinal <= $3`,
	`AND d.search_key LIKE ('%' || encode(convert_to(words.keyword,'UTF8'),'hex') || '%') COLLATE "C"
		AND p.publication_ordinal <= $3`, 1)

// Large encoded fields exceed PostgreSQL's statistics width limit. The planner
// can estimate a ubiquitous word at ten rows and perform 100k random lookups.
// A bounded page sample is ONLY a path-selection hint, never a result set or
// candidate limit: broad words scan all normalized documents with the original
// exact predicates, avoiding both bitmap lookups and source assembly. Sampling
// unfiltered documents avoids per-account caches and query text in telemetry.
func (c *Community) postSearchIndexSelective(ctx context.Context, tx pgx.Tx, keyword string) (bool, error) {
	defer readtiming.Span(ctx, "search_route")()
	var sampled, matched int
	err := tx.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE search_key LIKE
 ('%'||encode(convert_to(lower($1::text COLLATE pg_catalog."und-x-icu"),'UTF8'),'hex')||'%') COLLATE "C")
 FROM (SELECT search_key FROM c_posts.post_search_documents TABLESAMPLE SYSTEM(1) REPEATABLE(0) LIMIT 128) sampled`, keyword).Scan(&sampled, &matched)
	return sampled < 32 || matched*4 < sampled, err
}

// A conservative routing hint, never a matching rule. No minimum keyword length
// is introduced. Punctuation-only and short words scan normalized fields with
// the original exact predicates; they never use the broad candidate index.
func postSearchHasTrigram(keyword string) bool {
	run := 0
	for _, r := range keyword {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			run++
			if run >= 3 {
				return true
			}
		} else {
			run = 0
		}
	}
	return false
}
