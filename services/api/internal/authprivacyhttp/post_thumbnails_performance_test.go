package authprivacyhttp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/jpeg"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/google/uuid"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/authprivacy"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/postmedia"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/readtiming"
)

func TestThumbnailReuseHTTPSPerformancePostgres(t *testing.T) {
	if os.Getenv("POST_THUMBNAIL_PERFORMANCE") != "1" {
		t.Skip("explicit HTTPS performance run required")
	}
	script, e := filepath.Abs("../../media/validate_image.py")
	if e != nil {
		t.Fatal(e)
	}
	source, e := os.ReadFile(script)
	if e != nil {
		t.Fatal(e)
	}
	hash := sha256.Sum256(source)
	validator, e := postmedia.NewProcessValidator(os.Getenv("POST_MEDIA_PYTHON"), script, hex.EncodeToString(hash[:]))
	if e != nil {
		t.Fatal(e)
	}
	s := e2eNewServices(t, func(c *authprivacy.Community) *authprivacy.Community {
		enabled, e := c.WithPostImages(context.Background(), validator)
		if e != nil {
			t.Fatal(e)
		}
		return enabled
	})
	enablePreparedMediaHTTP(t, s)
	token, _ := e2ePrivacySignup(t, s, "thumbnail_perf_owner")
	reader, _ := e2ePrivacySignup(t, s, "thumbnail_perf_reader")
	identity := postHTTPSIdentity(t, s, token, "图片作者", 83)
	channel := postHTTPSChannel(t, s, token)
	picture := image.NewRGBA(image.Rect(0, 0, 1024, 768))
	rng := rand.New(rand.NewSource(424242))
	for i := 0; i < len(picture.Pix); i += 4 {
		v := rng.Uint32()
		picture.Pix[i] = byte(v)
		picture.Pix[i+1] = byte(v >> 8)
		picture.Pix[i+2] = byte(v >> 16)
		picture.Pix[i+3] = 255
	}
	var encoded bytes.Buffer
	if e = jpeg.Encode(&encoded, picture, &jpeg.Options{Quality: 87}); e != nil {
		t.Fatal(e)
	}
	original := encoded.Bytes()
	m, e := validator.Inspect(context.Background(), original)
	if e != nil {
		t.Fatal(e)
	}
	selection := postmedia.Selection{MediaID: uuid.NewString(), SHA256: m.SHA256, ByteLength: m.ByteLength, MIME: m.MIME, Width: m.Width, Height: m.Height}
	accepted := postHTTPSJSON(t, s, "PUT", "/api/v1/post-commands/"+httpTestEncoding(16, 84), token, "", map[string]any{
		"title": "缩略图性能", "body": "", "channelId": channel, "identityId": identity, "images": []postmedia.Selection{selection}}, 202)
	post, task := accepted["postId"].(string), accepted["taskId"].(string)
	base := "/api/v1/me/post-tasks/" + task + "/attempts/1/images/" + selection.MediaID
	grant := postHTTPSJSON(t, s, "POST", base+"/upload-grant", token, "", nil, 200)
	mediaHTTPSBytes(t, s, "PUT", base+"/bytes", "Upload "+grant["uploadToken"].(string), m.MIME, original, 204)
	if _, e = s.c.PublishAcceptedPosts(context.Background(), 100); e != nil {
		t.Fatal(e)
	}
	group := postHTTPSJSON(t, s, "GET", "/api/v1/posts/"+post+"/images", reader, "", nil, 200)
	content := group["images"].([]any)[0].(map[string]any)["contentVersionId"].(string)
	route := "/api/v1/posts/" + post + "/images/" + content + "/thumbnail"
	enabled, e := s.c.WithPostThumbnailReuse(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	*s.c = *enabled
	expected := mediaHTTPSBytes(t, s, "GET", route, "Bearer "+reader, "", nil, 200)
	if root := os.Getenv("POST_THUMBNAIL_DEVICE_ROOT"); root != "" {
		serveThumbnailDeviceFixture(t, s, validator, root, reader)
		return
	}
	measure := func(reuse bool) int64 {
		var next *authprivacy.Community
		var err error
		if reuse {
			next, err = s.c.WithPostThumbnailReuse(context.Background())
		} else {
			next, err = s.c.WithPostImages(context.Background(), validator)
		}
		if err != nil {
			t.Fatal(err)
		}
		*s.c = *next
		samples := make([]int64, 0, 40)
		for i := 0; i < 45; i++ {
			start := readtiming.SampleClock()
			data := mediaHTTPSBytes(t, s, "GET", route, "Bearer "+reader, "", nil, 200)
			end := readtiming.SampleClock()
			elapsed, valid := readtiming.ElapsedClock(start, end)
			if !valid || !bytes.Equal(data, expected) {
				t.Fatalf("HTTPS clock or bytes invalid: raw=%d goLower=%d goUpper=%d brackets=%d", elapsed, end.Before.Sub(start.After).Nanoseconds(), end.After.Sub(start.Before).Nanoseconds(), start.After.Sub(start.Before).Nanoseconds()+end.After.Sub(end.Before).Nanoseconds())
			}
			if i >= 5 {
				samples = append(samples, elapsed)
			}
		}
		sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
		return samples[37]
	}
	type round struct {
		Round         int     `json:"round"`
		BaselineP95NS int64   `json:"baselineP95NS"`
		ReuseP95NS    int64   `json:"reuseP95NS"`
		Reduction     float64 `json:"reduction"`
	}
	var rounds []round
	for i := 0; i < 3; i++ {
		var baseline, reuse int64
		if i%2 == 0 {
			baseline = measure(false)
			reuse = measure(true)
		} else {
			reuse = measure(true)
			baseline = measure(false)
		}
		rounds = append(rounds, round{i + 1, baseline, reuse, 1 - float64(reuse)/float64(baseline)})
	}
	raw, _ := json.Marshal(rounds)
	t.Log("THUMBNAIL_HTTPS_PERF", string(raw))
	for _, r := range rounds {
		if r.Reduction < .6 {
			t.Errorf("HTTPS round %d failed 60%% threshold: %.2f%%", r.Round, r.Reduction*100)
		}
	}
}
