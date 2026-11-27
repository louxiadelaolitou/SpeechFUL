package authprivacyhttp

import (
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/authprivacy"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/postmedia"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
)

func strictContinuationPreviews(window authprivacy.FeedWindowPreviews, response authprivacy.PostResponse, route postRoute) (any, error) {
	if !route.windowPreviews || route.windowDirection != "older" {
		return nil, errMalformed
	}
	base, err := strictPostValue(window.IncrementalFeedPage, route)
	if err != nil {
		return nil, err
	}
	window.IncrementalFeedPage = base.(posts.IncrementalFeedPage)
	if len(window.Items) == 0 {
		return nil, errMalformed
	}
	selected := postRoute{kind: authprivacy.PostInteractionBatch}
	for _, item := range window.Items {
		selected.ids = append(selected.ids, item.PostID)
	}
	if _, err := strictPostInteractions(window.Previews.PostInteractionPage, selected); err != nil {
		return nil, err
	}
	images := authprivacy.FeedPreviewImagePosts(window.Items)
	if window.Previews.ImageManifests == nil || len(window.Previews.ImageManifests) != len(images) {
		return nil, errMalformed
	}
	groups := make([]postmedia.Group, len(images))
	for i, group := range window.Previews.ImageManifests {
		if !validImageGroup(group, images[i], response.ServerTime, response.ExpiresAt) || len(group.Images) == 0 {
			return nil, errMalformed
		}
		groups[i] = withThumbnailSources(group)
	}
	window.Previews.ImageManifests = groups
	return window, nil
}
