package authprivacy

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/postmedia"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
)

type FeedWindowPreviews struct {
	posts.IncrementalFeedPage
	Previews FeedPreviews `json:"previews"`
}

type FeedPreviews struct {
	posts.PostInteractionPage
	ImageManifests []postmedia.Group `json:"imageManifests"`
}

func FeedPreviewImagePosts(items []posts.FeedPost) []string {
	ids := make([]string, 0, 3)
	for _, item := range items {
		if item.ImageCount > 0 {
			ids = append(ids, item.PostID)
			if len(ids) == 3 {
				break
			}
		}
	}
	return ids
}

func (c *Community) continuationPreviews(ctx context.Context, tx pgx.Tx, account uuid.UUID, at time.Time, page posts.IncrementalFeedPage, ids []string) (any, error) {
	child, err := tx.Begin(ctx)
	if err != nil {
		return nil, err
	}
	extras, err := readContinuationPreviews(ctx, child, account, at, ids, FeedPreviewImagePosts(page.Items))
	if err != nil {
		if rollbackErr := child.Rollback(ctx); rollbackErr != nil {
			return nil, rollbackErr
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return page, nil
	}
	if err = child.Commit(ctx); err != nil {
		return nil, err
	}
	return FeedWindowPreviews{IncrementalFeedPage: page, Previews: extras}, nil
}

func readContinuationPreviews(ctx context.Context, tx pgx.Tx, account uuid.UUID, at time.Time, ids, mediaIDs []string) (FeedPreviews, error) {
	interactions, err := readPostInteractions(ctx, tx, account, ids)
	if err != nil {
		return FeedPreviews{}, err
	}
	groups := make([]postmedia.Group, 0, len(mediaIDs))
	for _, post := range mediaIDs {
		snapshot, err := readPreparedPostMediaSnapshot(ctx, tx, post, account)
		if err != nil {
			return FeedPreviews{}, err
		}
		if !snapshot.visible || len(snapshot.items) == 0 {
			return FeedPreviews{}, ErrAuthorizationUnavailable
		}
		images := make([]postmedia.Image, len(snapshot.items))
		for i, item := range snapshot.items {
			images[i] = item.publicImage(post)
		}
		groups = append(groups, postmedia.Group{
			PostID: post, Images: images, AccessValidUntil: at.Add(time.Minute).UTC().Format(time.RFC3339Nano),
		})
	}
	return FeedPreviews{PostInteractionPage: interactions, ImageManifests: groups}, nil
}
