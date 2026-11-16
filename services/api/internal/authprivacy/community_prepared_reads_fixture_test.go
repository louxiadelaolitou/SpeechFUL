package authprivacy

import "testing"

// removePreparedReadFixture removes feed facts then comment facts from an
// owned newLab database when a test deliberately reconstructs an older schema.
// In particular, dropping c_posts alone leaves the public identity/account
// triggers calling c_comments.advance_read_versions against missing tables.
// This is fixture construction, never a runtime fallback or migration down.
func removePreparedReadFixture(t *testing.T, l *lab) {
	t.Helper()
	removePreparedFeedReadFixture(t, l)
	mustExec(t, l.cp, `DROP TRIGGER prepared_read_post ON c_posts.posts;
DROP TRIGGER prepared_read_task ON c_comments.tasks;
DROP TRIGGER prepared_read_attempt ON c_comments.attempts;
DROP TRIGGER prepared_read_image ON c_comments.images;
DROP TRIGGER prepared_read_like ON c_comments.likes;
DROP TRIGGER prepared_read_identity ON public.community_identities;
DROP TRIGGER prepared_read_account ON c_auth.accounts;
DROP TRIGGER prepared_read_block ON c_posts.blocked_authors;
DROP FUNCTION c_comments.advance_read_versions();
DROP TABLE c_comments.read_scope_versions,c_comments.read_post_versions,c_comments.read_comment_versions,
c_posts.read_identity_versions,c_posts.read_account_versions`)
}

func removePreparedFeedReadFixture(t *testing.T, l *lab) {
	t.Helper()
	removePostSearchDocumentFixture(t, l)
	mustExec(t, l.cp, `DROP TRIGGER prepared_feed_channel ON public.channels;
DROP TRIGGER prepared_feed_post ON c_posts.posts;
DROP TRIGGER prepared_feed_content ON c_posts.attempt_contents;
DROP TRIGGER prepared_feed_image ON c_posts.images;
DROP TRIGGER prepared_feed_edit_media ON c_posts.post_edit_media;
DROP FUNCTION c_posts.advance_feed_read_versions();
DROP TABLE c_posts.read_feed_scope_versions,c_posts.read_feed_post_versions`)
}

func removePostSearchDocumentFixture(t *testing.T, l *lab) {
	t.Helper()
	mustExec(t, l.cp, `DROP TRIGGER IF EXISTS post_search_document_post ON c_posts.posts;
DROP TRIGGER IF EXISTS post_search_document_content ON c_posts.attempt_contents;
DROP FUNCTION IF EXISTS c_posts.refresh_post_search_document(uuid),c_posts.sync_post_search_document(),
 c_posts.backfill_post_search_documents(integer),c_posts.validate_post_search_documents(),c_posts.current_post_search_documents(uuid);
DROP TABLE IF EXISTS c_posts.post_search_documents,c_posts.post_search_document_state`)
}
