-- Explicit opt-in after 0014, separately from the accepted text grants.
-- Execute only against a disposable/approved database as its migration owner.
REVOKE ALL ON c_posts.images FROM PUBLIC,hnuhole_c_runtime,hnuhole_c_recovery,hnuhole_business;
GRANT SELECT,INSERT ON c_posts.images TO hnuhole_c_runtime;
GRANT UPDATE(payload,verified_at,erased_at,display_width,display_height,frames,cycle_duration_ms,max_frame_duration_ms,grant_digest,grant_expires_at)
 ON c_posts.images TO hnuhole_c_runtime;

-- Optional 0043 derivative storage. Keep lifecycle hooks when reuse is off.
DO $$ BEGIN
 IF to_regclass('c_posts.thumbnails') IS NOT NULL THEN
  REVOKE ALL ON c_posts.thumbnails,c_posts.thumbnail_storage FROM PUBLIC,hnuhole_c_runtime,hnuhole_c_recovery,hnuhole_business;
  GRANT SELECT,INSERT,DELETE ON c_posts.thumbnails TO hnuhole_c_runtime;
  GRANT SELECT ON c_posts.thumbnail_storage TO hnuhole_c_runtime;
  -- pg_restore --no-privileges restores default PUBLIC EXECUTE. Reapply the
  -- trigger-only boundary as well as table grants before runtime startup.
  REVOKE ALL ON FUNCTION c_posts.account_thumbnail_storage(),c_posts.erase_source_thumbnails()
   FROM PUBLIC,hnuhole_c_runtime,hnuhole_c_recovery,hnuhole_business;
 END IF;
END $$;

-- Optional 0055 Redis deletion outbox. The SECURITY DEFINER trigger records
-- erasures; the runtime may only claim, complete, retry and retire its queue.
DO $$ BEGIN
 IF to_regclass('c_posts.thumbnail_cache_outbox') IS NOT NULL THEN
  REVOKE ALL ON c_posts.thumbnail_cache_outbox FROM PUBLIC,hnuhole_c_runtime,hnuhole_c_recovery,hnuhole_business;
  GRANT SELECT,UPDATE,DELETE ON c_posts.thumbnail_cache_outbox TO hnuhole_c_runtime;
  REVOKE ALL ON FUNCTION c_posts.record_thumbnail_cache_erasure()
   FROM PUBLIC,hnuhole_c_runtime,hnuhole_c_recovery,hnuhole_business;
 END IF;
END $$;
