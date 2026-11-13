package authprivacy

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
)

const PostHiddenState PostReadKind = "hiddenPostState"
const PostHiddenSet PostReadKind = "hiddenPostSet"

func (c *Community) hiddenPostState(ctx context.Context, tx pgx.Tx, account uuid.UUID, id string) (posts.HiddenPostState, string, error) {
	state := posts.HiddenPostState{PostID: id}
	var version string
	err := tx.QueryRow(ctx, `SELECT hidden,version::text FROM c_posts.hidden_posts WHERE account_id=$1 AND post_id=$2`, account, id).Scan(&state.Hidden, &version)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return state, "", err
	}
	encoded, err := json.Marshal([]string{"HNUHOLE/HIDDEN-POST/V1", account.String(), id, version})
	if err != nil {
		return state, "", err
	}
	mac := hmac.New(sha256.New, c.requestKey[:])
	_, _ = mac.Write(encoded)
	return state, `"` + hex.EncodeToString(mac.Sum(nil)) + `"`, nil
}

// A caller can read/restore a previously hidden post after it becomes unavailable
// but cannot probe inaccessible IDs that were never associated with its account.
func hiddenPostReadable(ctx context.Context, tx pgx.Tx, account uuid.UUID, id string) error {
	_, _, err := readPublicPost(ctx, tx, account, id)
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	var known bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM c_posts.hidden_posts WHERE account_id=$1 AND post_id=$2)`, account, id).Scan(&known); err != nil {
		return err
	}
	if !known {
		return postError("POST_NOT_FOUND", 404)
	}
	return nil
}

func (c *Community) ReadHiddenPost(ctx context.Context, bearer [32]byte, id string) (PostResponse, error) {
	if _, err := posts.ParseResourceID(id); err != nil {
		return PostResponse{}, err
	}
	if c.posts == nil || !c.posts.hiddenPostsEnabled {
		return PostResponse{}, ErrAuthorizationUnavailable
	}
	var tag string
	result, err := c.withPostSession(ctx, bearer, func(tx pgx.Tx, account uuid.UUID, _ AuthorizationDecision, _ restriction) (any, error) {
		if err := hiddenPostReadable(ctx, tx, account, id); err != nil {
			return nil, err
		}
		value, version, err := c.hiddenPostState(ctx, tx, account, id)
		tag = version
		return value, err
	})
	result.InteractionVersion = tag
	return result, err
}

// The conditional write fences late requests, including a hide arriving after
// undo. The account lock and final Safety Gate belong to withPostSession.
func (c *Community) SetHiddenPost(ctx context.Context, bearer [32]byte, id string, hidden bool, expected string) (PostResponse, error) {
	if _, err := posts.ParseResourceID(id); err != nil || !posts.ValidInteractionVersion(expected) {
		return PostResponse{}, posts.ErrInvalidIntent
	}
	if c.posts == nil || !c.posts.hiddenPostsEnabled {
		return PostResponse{}, ErrAuthorizationUnavailable
	}
	var tag string
	result, err := c.withPostSession(ctx, bearer, func(tx pgx.Tx, account uuid.UUID, _ AuthorizationDecision, _ restriction) (any, error) {
		if err := hiddenPostReadable(ctx, tx, account, id); err != nil {
			return nil, err
		}
		var locked string
		if err := tx.QueryRow(ctx, `SELECT post_id FROM c_posts.posts WHERE post_id=$1 FOR UPDATE`, id).Scan(&locked); err != nil {
			return nil, err
		}
		if hidden {
			if _, _, err := readPublicPost(ctx, tx, account, id); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return nil, postError("POST_NOT_FOUND", 404)
				}
				return nil, err
			}
		}
		previous, current, err := c.hiddenPostState(ctx, tx, account, id)
		if err != nil {
			return nil, err
		}
		if !hmac.Equal([]byte(current), []byte(expected)) {
			return nil, postError("INTERACTION_VERSION_CONFLICT", 412)
		}
		version, err := uuid.NewRandom()
		if err != nil {
			return nil, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO c_posts.hidden_posts(account_id,post_id,hidden,version) VALUES($1,$2,$3,$4)
 ON CONFLICT(account_id,post_id) DO UPDATE SET hidden=excluded.hidden,version=excluded.version`, account, id, hidden, version); err != nil {
			return nil, err
		}
		if previous.Hidden != hidden {
			// Search and feed cursors carry the same private preference revision.
			if _, err = tx.Exec(ctx, `INSERT INTO c_posts.feed_preferences(account_id,blocked_channel_ids,revision) VALUES($1,'{}',1)
 ON CONFLICT(account_id) DO UPDATE SET revision=c_posts.feed_preferences.revision+1`, account); err != nil {
				return nil, err
			}
		}
		value, next, err := c.hiddenPostState(ctx, tx, account, id)
		tag = next
		return value, err
	})
	result.InteractionVersion = tag
	return result, err
}
