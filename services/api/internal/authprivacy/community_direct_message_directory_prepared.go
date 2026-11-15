package authprivacy

import (
	"bytes"
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/readtiming"
)

// The directory is ordered by conversation_id and capped at 100. Each endpoint
// column gets its own index branch; only the merged page is projected.
const dmDirectoryPageLimit = 100

const dmDirectoryCandidatesSQL = `WITH viewer AS (
 SELECT 1 FROM public.community_identities
 WHERE identity_id=$1::uuid AND account_id=$2::uuid AND deleted_at IS NULL
), low_page AS (
 SELECT c.conversation_id FROM c_dm.conversations c CROSS JOIN viewer
 WHERE c.low_identity=$1::uuid AND ($3::uuid IS NULL OR c.conversation_id>$3::uuid)
 ORDER BY c.conversation_id LIMIT 100
), high_page AS (
 SELECT c.conversation_id FROM c_dm.conversations c CROSS JOIN viewer
 WHERE c.high_identity=$1::uuid AND ($3::uuid IS NULL OR c.conversation_id>$3::uuid)
 ORDER BY c.conversation_id LIMIT 100
)
SELECT conversation_id FROM low_page
UNION ALL
SELECT conversation_id FROM high_page
ORDER BY conversation_id LIMIT 100`

const dmDirectoryProjectionSQL = `SELECT c.conversation_id::text,i.identity_id::text,p.identity_id::text,
 CASE WHEN p.deleted_at IS NULL THEN p.nickname ELSE '注销身份 · '||(SELECT short_code FROM c_posts.identity_public_labels WHERE identity_id=p.identity_id) END,
 CASE WHEN p.deleted_at IS NULL THEN p.avatar ELSE 'inactive-v1' END,
 c.source_post::text,c.sequence,u.unread,
 CASE WHEN c.low_identity=i.identity_id THEN c.low_sent AND NOT c.high_sent ELSE c.high_sent AND NOT c.low_sent END,
 p.deleted_at IS NULL AND pa.state='ACTIVE' AND NOT c_posts.accounts_blocked(i.account_id,p.account_id) AND NOT EXISTS(SELECT 1 FROM c_dm.blocks b WHERE (b.owner_account=i.account_id AND b.peer_account=p.account_id) OR (b.owner_account=p.account_id AND b.peer_account=i.account_id)),
 COALESCE((SELECT b.block_id::text FROM c_posts.blocked_authors b WHERE b.account_id=i.account_id AND b.author_account_id=p.account_id),''),
 c.revision,COALESCE(s.version,0),COALESCE(s.pinned,false),COALESCE(s.muted,false),
 COALESCE(s.hidden_through,0)>=c.sequence AND COALESCE(s.hidden_through,0)>0,
 c.sequence<=COALESCE(s.deleted_through,0),COALESCE(s.deleted_through,0),COALESCE(s.note,''),
 CASE WHEN source.visibility='PUBLISHED' AND NOT source.moderation_hidden THEN COALESCE(source_content.title,'') ELSE '' END,
 u.acknowledged,c.low_sent AND c.high_sent
 FROM c_dm.conversations c
 JOIN public.community_identities i ON i.identity_id=$2::uuid AND i.account_id=$3::uuid
 JOIN public.community_identities p ON p.identity_id=CASE WHEN i.identity_id=c.low_identity THEN c.high_identity ELSE c.low_identity END
 JOIN c_auth.accounts pa ON pa.account_id=p.account_id
 LEFT JOIN c_posts.posts source ON source.post_id=c.source_post
 LEFT JOIN c_posts.publication_tasks source_task ON source_task.post_id=source.post_id
 LEFT JOIN c_posts.attempt_contents source_content ON source_content.task_id=source_task.task_id AND source_content.version=source.published_attempt_version AND source_content.erased_at IS NULL
 LEFT JOIN c_dm.settings s ON s.conversation_id=c.conversation_id AND s.identity_id=i.identity_id
 LEFT JOIN LATERAL (
  SELECT count(*) AS unread,count(*) FILTER (WHERE m.acknowledged) AS acknowledged
  FROM c_dm.messages m
  WHERE m.conversation_id=c.conversation_id AND m.recipient=i.identity_id
    AND NOT m.read AND NOT m.retracted AND m.sequence>COALESCE(s.deleted_through,0)
 ) u ON true
 WHERE c.conversation_id=ANY($1::uuid[]) AND (c.low_identity=$2::uuid OR c.high_identity=$2::uuid)
 ORDER BY c.conversation_id`

// The proof binds the page's conversation revisions, both endpoint identities,
// the endpoint accounts, source posts and page-bounded DM blocks. The viewer
// itself may legitimately be absent (deleted identity) and is then omitted from
// the require-version checks.
const dmDirectoryVersionsSQL = `WITH page AS (
 SELECT unnest($3::uuid[]) AS conversation_id
), conversations AS (
 SELECT c.conversation_id,c.revision,c.source_post,c.low_identity,c.high_identity
 FROM c_dm.conversations c JOIN page USING(conversation_id)
), peers AS (
 SELECT CASE WHEN c.low_identity=$2::uuid THEN c.high_identity ELSE c.low_identity END AS identity_id
 FROM conversations c
), endpoints AS (
 SELECT $2::uuid AS identity_id
 UNION
 SELECT identity_id FROM peers
), peer_accounts AS (
 SELECT i.account_id FROM public.community_identities i WHERE i.identity_id IN (SELECT identity_id FROM peers)
), accounts AS (
 SELECT $1::uuid AS account_id
 UNION
 SELECT account_id FROM peer_accounts
), posts AS (
 SELECT DISTINCT source_post AS post_id FROM conversations WHERE source_post IS NOT NULL
), blocks AS (
 SELECT b.owner_account,b.peer_account FROM c_dm.blocks b
 WHERE (b.owner_account=$1::uuid AND b.peer_account IN (SELECT account_id FROM peer_accounts))
    OR (b.peer_account=$1::uuid AND b.owner_account IN (SELECT account_id FROM peer_accounts))
)
SELECT jsonb_build_object(
 'viewer',(SELECT jsonb_build_object('account',i.account_id,'deleted',i.deleted_at,'version',v.projection_revision)
   FROM public.community_identities i LEFT JOIN c_posts.read_identity_versions v USING(identity_id)
   WHERE i.identity_id=$2::uuid),
 'conversations',(SELECT coalesce(jsonb_agg(jsonb_build_array(c.conversation_id,c.revision,c.source_post,c.low_identity,c.high_identity) ORDER BY c.conversation_id),'[]'::jsonb) FROM conversations c),
 'identities',(SELECT coalesce(jsonb_agg(jsonb_build_array(i.identity_id,i.account_id,i.nickname,i.avatar,i.deleted_at,v.projection_revision,l.short_code) ORDER BY i.identity_id),'[]'::jsonb)
   FROM public.community_identities i
   JOIN endpoints e ON e.identity_id=i.identity_id
   LEFT JOIN c_posts.read_identity_versions v ON v.identity_id=i.identity_id
   LEFT JOIN c_posts.identity_public_labels l ON l.identity_id=i.identity_id),
 'accounts',(SELECT coalesce(jsonb_agg(jsonb_build_array(a.account_id,v.projection_revision,v.visibility_revision) ORDER BY a.account_id),'[]'::jsonb)
   FROM c_auth.accounts a JOIN accounts x ON x.account_id=a.account_id
   LEFT JOIN c_posts.read_account_versions v ON v.account_id=a.account_id),
 'posts',(SELECT coalesce(jsonb_agg(jsonb_build_array(p.post_id,v.projection_revision) ORDER BY p.post_id),'[]'::jsonb)
   FROM c_posts.posts p JOIN posts x ON x.post_id=p.post_id
   LEFT JOIN c_posts.read_feed_post_versions v ON v.post_id=p.post_id),
 'dm_blocks',(SELECT coalesce(jsonb_agg(jsonb_build_array(b.owner_account,b.peer_account) ORDER BY b.owner_account,b.peer_account),'[]'::jsonb) FROM blocks b)
), NOT EXISTS (SELECT 1 FROM page p LEFT JOIN c_dm.conversations c USING(conversation_id) WHERE c.conversation_id IS NULL)
 AND NOT EXISTS (SELECT 1 FROM endpoints e JOIN public.community_identities i USING(identity_id) LEFT JOIN c_posts.read_identity_versions v USING(identity_id) WHERE v.identity_id IS NULL)
 AND NOT EXISTS (SELECT 1 FROM accounts x JOIN c_auth.accounts a USING(account_id) LEFT JOIN c_posts.read_account_versions v USING(account_id) WHERE v.account_id IS NULL)
 AND NOT EXISTS (SELECT 1 FROM posts x JOIN c_posts.posts p USING(post_id) LEFT JOIN c_posts.read_feed_post_versions v USING(post_id) WHERE v.post_id IS NULL)`

// Any eligible conversation inside the returned id interval that was not part
// of the prepared page changes the page, even if its UUID sorts before a later
// conversation created afterwards.
const dmDirectoryRangeSQL = `WITH viewer AS (
 SELECT 1 FROM public.community_identities
 WHERE identity_id=$2::uuid AND account_id=$1::uuid AND deleted_at IS NULL
), eligible AS (
 SELECT c.conversation_id FROM c_dm.conversations c CROSS JOIN viewer WHERE c.low_identity=$2::uuid
 UNION ALL
 SELECT c.conversation_id FROM c_dm.conversations c CROSS JOIN viewer WHERE c.high_identity=$2::uuid
)
SELECT EXISTS(
 SELECT 1 FROM eligible e
 WHERE ($3::uuid IS NULL OR e.conversation_id>$3::uuid)
   AND ($4::uuid IS NULL OR e.conversation_id<=$4::uuid)
   AND NOT (e.conversation_id=ANY($5::uuid[]))
)`

type dmDirectoryReadProof struct {
	identity string
	after    string
	last     string
	ids      []string
	versions []byte
}

func (p *dmDirectoryReadProof) read(ctx context.Context, tx pgx.Tx, account uuid.UUID) ([]byte, error) {
	var proof []byte
	var intact bool
	if e := tx.QueryRow(ctx, dmDirectoryVersionsSQL, account, p.identity, p.ids).Scan(&proof, &intact); e != nil {
		return nil, e
	}
	if !intact {
		return nil, ErrAuthorizationUnavailable
	}
	return proof, nil
}

func (p *dmDirectoryReadProof) revalidate(ctx context.Context, tx pgx.Tx, account uuid.UUID) error {
	current, e := p.read(ctx, tx, account)
	if e != nil {
		return e
	}
	if !bytes.Equal(current, p.versions) {
		return errReadConflict
	}
	var fresh bool
	if e = tx.QueryRow(ctx, dmDirectoryRangeSQL, account, p.identity, nullableDMUUIDText(p.after), nullableDMUUIDText(p.last), p.ids).Scan(&fresh); e != nil {
		return e
	}
	if fresh {
		return errReadConflict
	}
	return nil
}

func nullableDMUUIDText(id string) any {
	if id == "" {
		return nil
	}
	return id
}

func (c *Community) readDMChatDirectoryPrepared(ctx context.Context, bearer [32]byte, identity, after string) (PostResponse, error) {
	ctx = readtiming.Phase(ctx, "dm-directory")
	return c.withPreparedReadPlan(ctx, bearer, postError("SERVICE_UNAVAILABLE", 503), func(prepareCtx context.Context, tx pgx.Tx, s readSubject) (*preparedReadPlan, error) {
		proof := &dmDirectoryReadProof{identity: identity, after: after, ids: []string{}}
		items := make([]DMChatConversation, 0)
		var pending error
		rows, e := tx.Query(prepareCtx, dmDirectoryCandidatesSQL, identity, s.account, nullableDMUUIDText(after))
		if e != nil {
			return nil, e
		}
		for rows.Next() {
			var id string
			if e = rows.Scan(&id); e != nil {
				rows.Close()
				return nil, e
			}
			proof.ids = append(proof.ids, id)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return nil, e
		}
		if len(proof.ids) > 0 {
			proof.last = proof.ids[len(proof.ids)-1]
			items, e = readDMChatDirectoryPage(prepareCtx, tx, identity, s.account, proof.ids)
			if e != nil {
				pending = e
			}
		}
		proof.versions, e = proof.read(prepareCtx, tx, s.account)
		if e != nil {
			pending = e
		}
		plan := &preparedReadPlan{
			Revalidate: func(tx pgx.Tx, a uuid.UUID, _ AuthorizationDecision, _ restriction) error {
				if len(proof.ids) > dmDirectoryPageLimit {
					return ErrAuthorizationUnavailable
				}
				return proof.revalidate(ctx, tx, a)
			},
			Complete: func(_ pgx.Tx, _ uuid.UUID, d AuthorizationDecision, m restriction) (any, error) {
				out := make([]DMChatConversation, len(items))
				copy(out, items)
				for i := range out {
					out[i].CanShare = out[i].CanShare && out[i].CanSend && !mutedAt(m, d.TrustedAt)
				}
				return DMChatDirectory{Items: out}, nil
			},
		}
		return plan, pending
	})
}

func readDMChatDirectoryPage(ctx context.Context, tx pgx.Tx, identity string, account uuid.UUID, ids []string) ([]DMChatConversation, error) {
	items := make([]DMChatConversation, 0, len(ids))
	rows, e := tx.Query(ctx, dmDirectoryProjectionSQL, ids, identity, account)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		v, e := scanDMChatConversation(rows)
		if e != nil {
			return nil, e
		}
		items = append(items, v)
	}
	return items, rows.Err()
}

func scanDMChatConversation(rows pgx.Rows) (DMChatConversation, error) {
	var v DMChatConversation
	e := rows.Scan(&v.ID, &v.IdentityID, &v.PeerID, &v.PeerName, &v.PeerAvatar, &v.SourcePostID, &v.Sequence, &v.Unread, &v.Waiting, &v.CanSend, &v.OwnBlockID, &v.Version, &v.SettingsVersion, &v.Pinned, &v.Muted, &v.Hidden, &v.Deleted, &v.DeletedThrough, &v.Note, &v.SourceTitle, &v.UnavailableUnread, &v.CanShare)
	return v, e
}
