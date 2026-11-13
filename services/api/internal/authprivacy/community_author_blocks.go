package authprivacy

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
)

type authorBlockRow struct {
	id        string
	state     string
	nickname  string
	avatar    string
	shortCode string
	createdAt time.Time
}

func (r authorBlockRow) public() posts.BlockedAuthor {
	return posts.BlockedAuthor{
		BlockID:   r.id,
		Author:    posts.PublicAuthor{State: r.state, Nickname: r.nickname, Avatar: r.avatar, ShortCode: r.shortCode},
		BlockedAt: r.createdAt.UTC(),
	}
}

// readPostAuthorAccount resolves the account behind a readable post without
// applying the viewer's block predicate. Repeating or retrying a block after
// the author became invisible must stay idempotent and cannot probe posts that
// were never public.
func readPostAuthorAccount(ctx context.Context, tx pgx.Tx, postID string) (uuid.UUID, posts.PublicAuthor, error) {
	var account uuid.UUID
	var accountState string
	var deleted *time.Time
	var nickname, avatar *string
	var short string
	err := tx.QueryRow(ctx, `SELECT p.owner_account_id,a.state,i.deleted_at,i.nickname,i.avatar,l.short_code
 FROM c_posts.posts p JOIN c_posts.publication_tasks t ON t.post_id=p.post_id
 JOIN c_posts.attempt_contents x ON x.task_id=t.task_id AND x.version=p.published_attempt_version
 JOIN public.community_identities i ON i.identity_id=t.identity_id
 JOIN c_auth.accounts a ON a.account_id=p.owner_account_id
 JOIN c_posts.identity_public_labels l ON l.identity_id=i.identity_id
 WHERE p.post_id=$1 AND p.visibility='PUBLISHED' AND NOT COALESCE((to_jsonb(p)->>'moderation_hidden')::boolean,false) AND x.erased_at IS NULL`, postID).
		Scan(&account, &accountState, &deleted, &nickname, &avatar, &short)
	if err != nil {
		return uuid.Nil, posts.PublicAuthor{}, err
	}
	author, err := projectPublicPostAuthor(accountState, deleted, nickname, avatar, short)
	return account, author, err
}

func readAuthorBlock(ctx context.Context, tx pgx.Tx, account, author uuid.UUID) (authorBlockRow, bool, error) {
	var row authorBlockRow
	err := tx.QueryRow(ctx, `SELECT block_id::text,author_state,nickname,avatar,short_code,created_at
 FROM c_posts.blocked_authors WHERE account_id=$1 AND author_account_id=$2`, account, author).
		Scan(&row.id, &row.state, &row.nickname, &row.avatar, &row.shortCode, &row.createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return authorBlockRow{}, false, nil
	}
	return row, err == nil, err
}

// bumpVisibilityRevision invalidates feed and search cursors on both sides of
// an account block. Sorted execution keeps concurrent A->B and B->A requests
// from deadlocking on the same preference rows.
func bumpVisibilityRevision(ctx context.Context, tx pgx.Tx, accounts ...uuid.UUID) error {
	slices.SortFunc(accounts, func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) })
	for _, account := range accounts {
		if _, err := tx.Exec(ctx, `INSERT INTO c_posts.feed_preferences(account_id,blocked_channel_ids,revision) VALUES($1,'{}',1)
 ON CONFLICT(account_id) DO UPDATE SET revision=c_posts.feed_preferences.revision+1`, account); err != nil {
			return err
		}
	}
	return nil
}

// lockAuthorAccountRow takes the weakest row lock the blocked_authors and
// feed_preferences foreign keys request on the author account. It has to run
// before the Gate row is locked (see postSessionPrepare); otherwise a target
// account's request, which locks its own account and then waits for the Gate,
// forms a lock cycle with this transaction and PostgreSQL aborts one side with
// SQLSTATE 40P01.
func lockAuthorAccountRow(ctx context.Context, tx pgx.Tx, account uuid.UUID) (bool, error) {
	var one int
	err := tx.QueryRow(ctx, `SELECT 1 FROM c_auth.accounts WHERE account_id=$1 FOR KEY SHARE`, account).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

const postDeadlockRetries = 3

// retryPostDeadlock retries a symmetric account block or unblock after
// PostgreSQL resolves an A-blocks-B / B-blocks-A cycle. Both attempts take
// their caller's account before the peer's, so the loser of that race is
// rolled back without business effects and the retry runs after the winner.
func retryPostDeadlock(ctx context.Context, attempt func() (PostResponse, error)) (PostResponse, error) {
	for retry := 0; ; retry++ {
		response, err := attempt()
		var pgErr *pgconn.PgError
		if err == nil || retry == postDeadlockRetries-1 || !errors.As(err, &pgErr) || pgErr.Code != "40P01" {
			return response, err
		}
		select {
		case <-ctx.Done():
			return PostResponse{}, err
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// BlockPostAuthor stores an account-level block anchored by a source post.
// Every identity of the author account is covered by the symmetric read
// predicate; the stored row keeps only the public projection snapshot.
func (c *Community) BlockPostAuthor(ctx context.Context, bearer [32]byte, postID string) (PostResponse, error) {
	if _, err := posts.ParseResourceID(postID); err != nil {
		return PostResponse{}, err
	}
	var targetAccount uuid.UUID
	var target posts.PublicAuthor
	// prepare only resolves and locks; business outcomes are replayed by the
	// action after the Gate validates the session, so a revoked token learns
	// nothing about the anchor's existence or owner.
	var prepareErr error
	return retryPostDeadlock(ctx, func() (PostResponse, error) {
		targetAccount, target, prepareErr = uuid.Nil, posts.PublicAuthor{}, nil
		return c.withPostSessionPrepared(ctx, bearer, func(ctx context.Context, tx pgx.Tx, account uuid.UUID) error {
			authorAccount, author, err := readPostAuthorAccount(ctx, tx, postID)
			if errors.Is(err, pgx.ErrNoRows) {
				prepareErr = postError("POST_NOT_FOUND", 404)
				return nil
			}
			if err != nil {
				return err
			}
			if authorAccount == account {
				prepareErr = postError("SELF_BLOCK_REJECTED", 409)
				return nil
			}
			found, err := lockAuthorAccountRow(ctx, tx, authorAccount)
			if err != nil {
				return err
			}
			if !found {
				prepareErr = postError("POST_NOT_FOUND", 404)
				return nil
			}
			targetAccount, target = authorAccount, author
			return nil
		}, func(tx pgx.Tx, account uuid.UUID, final AuthorizationDecision, _ restriction) (any, error) {
			if prepareErr != nil {
				return nil, prepareErr
			}
			if targetAccount == uuid.Nil {
				return nil, ErrAuthorizationUnavailable
			}
			authorAccount, author := targetAccount, target
			// A concurrent unblock may remove a conflicting row between the read
			// and the insert; retry once with fresh visibility.
			for range 2 {
				existing, found, err := readAuthorBlock(ctx, tx, account, authorAccount)
				if err != nil {
					return nil, err
				}
				if found {
					item := existing.public()
					return posts.AuthorBlockReceipt{BlockID: item.BlockID, State: "BLOCKED", Author: item.Author, BlockedAt: item.BlockedAt}, nil
				}
				blockID, err := uuid.NewRandom()
				if err != nil {
					return nil, err
				}
				var inserted string
				err = tx.QueryRow(ctx, `INSERT INTO c_posts.blocked_authors(block_id,account_id,author_account_id,author_state,nickname,avatar,short_code,created_at)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8)
 ON CONFLICT(account_id,author_account_id) DO NOTHING
 RETURNING block_id::text`, blockID, account, authorAccount, author.State, author.Nickname, author.Avatar, author.ShortCode, final.TrustedAt).
					Scan(&inserted)
				if err == nil {
					if err = bumpVisibilityRevision(ctx, tx, account, authorAccount); err != nil {
						return nil, err
					}
					return posts.AuthorBlockReceipt{BlockID: inserted, State: "BLOCKED", Author: author, BlockedAt: final.TrustedAt.UTC()}, nil
				}
				if !errors.Is(err, pgx.ErrNoRows) {
					return nil, err
				}
			}
			return nil, postError("SERVICE_UNAVAILABLE", 503)
		})
	})
}

// authorBlockListLimit bounds the private, self-created list without
// pagination; clients reject larger pages rather than silently truncating.
const authorBlockListLimit = 200

func (c *Community) AuthorBlocks(ctx context.Context, bearer [32]byte) (PostResponse, error) {
	return c.withPostSession(ctx, bearer, func(tx pgx.Tx, account uuid.UUID, _ AuthorizationDecision, _ restriction) (any, error) {
		rows, err := tx.Query(ctx, `SELECT block_id::text,author_state,nickname,avatar,short_code,created_at
 FROM c_posts.blocked_authors WHERE account_id=$1 ORDER BY created_at DESC,block_id DESC LIMIT $2`, account, authorBlockListLimit)
		if err != nil {
			return nil, err
		}
		items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (posts.BlockedAuthor, error) {
			var block authorBlockRow
			err := row.Scan(&block.id, &block.state, &block.nickname, &block.avatar, &block.shortCode, &block.createdAt)
			return block.public(), err
		})
		if items == nil {
			items = []posts.BlockedAuthor{}
		}
		return posts.BlockedAuthorPage{Items: items}, err
	})
}

// UnblockPostAuthor is idempotent: a record owned by another account is
// indistinguishable from an already removed one.
func (c *Community) UnblockPostAuthor(ctx context.Context, bearer [32]byte, blockID string) (PostResponse, error) {
	id, err := posts.ParseResourceID(blockID)
	if err != nil {
		return PostResponse{}, err
	}
	var targetAccount uuid.UUID
	return retryPostDeadlock(ctx, func() (PostResponse, error) {
		targetAccount = uuid.Nil
		return c.withPostSessionPrepared(ctx, bearer, func(ctx context.Context, tx pgx.Tx, account uuid.UUID) error {
			err := tx.QueryRow(ctx, `SELECT author_account_id FROM c_posts.blocked_authors WHERE block_id=$1 AND account_id=$2`, id, account).Scan(&targetAccount)
			if errors.Is(err, pgx.ErrNoRows) {
				targetAccount = uuid.Nil
				return nil
			}
			if err != nil {
				return err
			}
			_, err = lockAuthorAccountRow(ctx, tx, targetAccount)
			return err
		}, func(tx pgx.Tx, account uuid.UUID, _ AuthorizationDecision, _ restriction) (any, error) {
			var authorAccount uuid.UUID
			err := tx.QueryRow(ctx, `DELETE FROM c_posts.blocked_authors WHERE block_id=$1 AND account_id=$2 RETURNING author_account_id`, id, account).Scan(&authorAccount)
			result := posts.AuthorBlockRemoval{BlockID: blockID, State: "ABSENT"}
			if errors.Is(err, pgx.ErrNoRows) {
				return result, nil
			}
			if err != nil {
				return nil, err
			}
			if err = bumpVisibilityRevision(ctx, tx, account, authorAccount); err != nil {
				return nil, err
			}
			result.State = "REMOVED"
			return result, nil
		})
	})
}
