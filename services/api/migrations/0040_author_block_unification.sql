-- +goose Up
-- DM blocks are no longer a separate mode. Copy every legacy row into the
-- account-level author block table, keeping the original block_id. The earliest
-- explicit choice wins when one account blocked several identities of the same
-- peer.
INSERT INTO c_posts.blocked_authors(block_id,account_id,author_account_id,author_state,nickname,avatar,short_code,created_at)
SELECT b.block_id,b.owner_account,b.peer_account,
       CASE WHEN p.deleted_at IS NULL THEN 'ACTIVE' ELSE 'DELETED' END,
       CASE WHEN p.deleted_at IS NULL THEN COALESCE(p.nickname,'') ELSE '' END,
       CASE WHEN p.deleted_at IS NULL THEN COALESCE(p.avatar,'default-v1') ELSE 'inactive-v1' END,
       COALESCE(l.short_code,''),
       b.created_at
FROM c_dm.blocks b
JOIN c_auth.accounts oa ON oa.account_id=b.owner_account AND oa.state<>'CLOSED'
JOIN c_auth.accounts pa ON pa.account_id=b.peer_account AND pa.state<>'CLOSED'
JOIN public.community_identities p ON p.identity_id=b.peer_id
LEFT JOIN c_posts.identity_public_labels l ON l.identity_id=p.identity_id
WHERE b.owner_account<>b.peer_account
ORDER BY b.created_at,b.block_id
ON CONFLICT DO NOTHING;

-- The unified list and every read predicate now read blocked_authors. Keeping
-- legacy rows would enforce a block that the blacklist cannot manage.
DELETE FROM c_dm.blocks;

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    RAISE EXCEPTION '0040 merges DM blocks into account-level author blocks; repair forward';
END $$;
-- +goose StatementEnd
