package authprivacyruntime

import (
	"strings"
	"testing"

	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/authprivacyhttp"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/postthumbnailcache"
)

func TestPostImagesRequiresExplicitCommunityConfiguration(t *testing.T) {
	for _, c := range []Config{
		{Environment: "dev", PostImages: &PostImagesConfig{}},
		{Environment: "dev", PostImages: &PostImagesConfig{PythonFile: "python", ValidatorFile: "validate.py", ValidatorSHA256: strings.Repeat("a", 64)}},
		{Environment: "dev", Posts: &PostsConfig{}, PostImages: &PostImagesConfig{PythonFile: "python", ValidatorFile: "validate.py", ValidatorSHA256: "short"}},
	} {
		if e := c.Validate(authprivacyhttp.CommunityService); e == nil || !strings.Contains(e.Error(), "postImages") {
			t.Fatal("incomplete postImages policy not rejected before material loading")
		}
	}
	c := Config{Environment: "dev", Posts: &PostsConfig{}, PostImages: &PostImagesConfig{PythonFile: "python", ValidatorFile: "validate.py", ValidatorSHA256: strings.Repeat("a", 64)}}
	if e := c.Validate(authprivacyhttp.VerifierService); e == nil || !strings.Contains(e.Error(), "postImages") {
		t.Fatal("V acquired image policy")
	}
}

func TestThumbnailReuseRequiresPreparedMedia(t *testing.T) {
	c := Config{Environment: "dev", Posts: &PostsConfig{}, PostImages: &PostImagesConfig{
		PythonFile: "python", ValidatorFile: "validate.py", ValidatorSHA256: strings.Repeat("a", 64), ThumbnailReuse: true}}
	if e := c.Validate(authprivacyhttp.CommunityService); e == nil || !strings.Contains(e.Error(), "preparedMediaReads") {
		t.Fatal("reuse accepted without final prepared-media proof", e)
	}
}

func TestThumbnailRedisRequiresReuseAndBoundNamespace(t *testing.T) {
	redis := &postthumbnailcache.Config{Address: "127.0.0.1:6380", Username: "community_thumbnail_cache",
		PasswordFile: "thumbnail-password", Namespace: "hnuhole:post-thumbnail:v1:dev:"}
	base := Config{Environment: "dev", Posts: &PostsConfig{PreparedMediaReads: true}, PostImages: &PostImagesConfig{
		PythonFile: "python", ValidatorFile: "validate.py", ValidatorSHA256: strings.Repeat("a", 64), ThumbnailRedis: redis}}
	if e := base.Validate(authprivacyhttp.CommunityService); e == nil || !strings.Contains(e.Error(), "thumbnail") {
		t.Fatal("redis cache accepted without reuse", e)
	}
	base.PostImages.ThumbnailReuse = true
	redis.Namespace = "hnuhole:post-thumbnail:v1:prod:"
	if e := base.Validate(authprivacyhttp.CommunityService); e == nil || !strings.Contains(e.Error(), "invalid post thumbnail redis configuration") {
		t.Fatal("cross-environment namespace accepted", e)
	}
}
