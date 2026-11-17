-- +goose Up
-- +goose StatementBegin
-- Notification list and unread-count reads decided unread with one EXISTS probe
-- into c_inbox.like_events per like notification. Keep the newest live event
-- version on the record row so the read path is a row-local comparison; the
-- sender trigger maintains it under the same record row lock as version and
-- updated_at. Install with source writes quiesced for the same transactional
-- migration chain as 0041/0042, where CREATE INDEX CONCURRENTLY is not
-- permitted.
LOCK TABLE c_inbox.records, c_inbox.like_events IN SHARE ROW EXCLUSIVE MODE;

ALTER TABLE c_inbox.records ADD COLUMN latest_like_version bigint NOT NULL DEFAULT 0
    CONSTRAINT records_latest_like_version_check
    CHECK (latest_like_version >= 0 AND latest_like_version <= version);

UPDATE c_inbox.records n
SET latest_like_version = m.max_version
FROM (
    SELECT notification_id, max(version) AS max_version
    FROM c_inbox.like_events GROUP BY notification_id
) m
WHERE m.notification_id = n.notification_id AND n.kind <> 'COMMENT';

CREATE OR REPLACE FUNCTION c_inbox.like_changed() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$
DECLARE actor uuid; content uuid; post uuid; owner_id uuid; identity uuid; kind_name text; notice uuid; next_version bigint; at_time timestamptz;
BEGIN
 IF TG_OP='DELETE' THEN actor=OLD.account_id; ELSE actor=NEW.account_id; END IF;
 IF TG_TABLE_SCHEMA='c_posts' THEN
  kind_name='LIKE_POST';
  IF TG_OP='DELETE' THEN content=OLD.post_id; ELSE content=NEW.post_id; END IF;
  SELECT p.post_id,p.owner_account_id,t.identity_id INTO post,owner_id,identity
   FROM c_posts.posts p JOIN c_posts.publication_tasks t USING(post_id) WHERE p.post_id=content;
 ELSE
  kind_name='LIKE_COMMENT';
  IF TG_OP='DELETE' THEN content=OLD.comment_id; ELSE content=NEW.comment_id; END IF;
  SELECT t.post_id,t.owner_account_id,t.identity_id INTO post,owner_id,identity FROM c_comments.tasks t WHERE t.comment_id=content;
 END IF;
 IF owner_id IS NULL OR actor=owner_id THEN RETURN NULL; END IF;
 IF TG_OP='DELETE' THEN
  -- A post/comment has one publication identity, so one record per content.
  -- Lock it before touching events so a concurrent like cannot be hidden by a
  -- stale recomputation.
  SELECT n.notification_id INTO notice FROM c_inbox.records n
   WHERE n.account_id=owner_id AND n.kind=kind_name AND n.content_id=content
   FOR UPDATE;
  IF notice IS NULL THEN RETURN NULL; END IF;
  DELETE FROM c_inbox.like_events WHERE notification_id=notice AND liker_account_id=actor;
  UPDATE c_inbox.records n SET latest_like_version=COALESCE(
   (SELECT max(e.version) FROM c_inbox.like_events e WHERE e.notification_id=notice),0)
   WHERE n.notification_id=notice;
  RETURN NULL;
 END IF;
 IF NOT EXISTS(SELECT 1 FROM public.community_identities i JOIN c_auth.accounts a USING(account_id)
  WHERE i.identity_id=identity AND i.deleted_at IS NULL AND a.state='ACTIVE') THEN RETURN NULL; END IF;
 at_time=clock_timestamp();
 INSERT INTO c_inbox.records(account_id,identity_id,post_id,comment_id,content_id,kind,created_at,updated_at,latest_like_version)
  VALUES(owner_id,identity,post,CASE WHEN kind_name='LIKE_COMMENT' THEN content END,content,kind_name,at_time,at_time,1)
  ON CONFLICT(account_id,identity_id,kind,content_id) DO UPDATE SET version=c_inbox.records.version+1,latest_like_version=c_inbox.records.version+1,updated_at=EXCLUDED.updated_at
  RETURNING notification_id,version INTO notice,next_version;
 INSERT INTO c_inbox.like_events VALUES(notice,actor,next_version);
 RETURN NULL;
END $$;

-- Account closure deletes a liker's events directly, so it must recompute the
-- same denormalized version; otherwise the owner would keep a phantom unread.
CREATE OR REPLACE FUNCTION c_inbox.account_closed() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$
DECLARE touched uuid[];
BEGIN
 IF NEW.state='CLOSED' AND OLD.state IS DISTINCT FROM NEW.state THEN
  touched := ARRAY(SELECT DISTINCT notification_id FROM c_inbox.like_events WHERE liker_account_id=NEW.account_id);
  -- Serialize with the like trigger's record row lock, then delete and recompute
  -- in separate statements so both see the newest committed events.
  PERFORM 1 FROM c_inbox.records WHERE notification_id=ANY(touched) ORDER BY notification_id FOR UPDATE;
  DELETE FROM c_comments.likes WHERE account_id=NEW.account_id;
  DELETE FROM c_inbox.like_events WHERE liker_account_id=NEW.account_id;
  UPDATE c_inbox.records n SET latest_like_version=COALESCE(
   (SELECT max(e.version) FROM c_inbox.like_events e WHERE e.notification_id=n.notification_id),0)
  WHERE n.notification_id=ANY(touched);
  DELETE FROM c_inbox.records WHERE account_id=NEW.account_id;
 END IF;
 RETURN NULL;
END $$;

-- One partial index serves both unread badge aggregation and the unread-only
-- page order. Rows leave it as soon as read_version catches the newest event.
CREATE INDEX inbox_unread ON c_inbox.records(account_id,identity_id,updated_at DESC,notification_id DESC)
    INCLUDE (kind,is_reply,read_version,latest_like_version)
    WHERE (kind='COMMENT' AND read_version=0)
       OR (kind<>'COMMENT' AND latest_like_version>read_version);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Forward-only like 0028/0041/0042: reads now require the denormalized version
-- and its index. Restoring the old function would silently restore per-row
-- event probes; abandon the access path with an explicit forward repair.
DO $$ BEGIN RAISE EXCEPTION 'notification unread index requires forward repair; Down is forbidden' END $$;
-- +goose StatementEnd
