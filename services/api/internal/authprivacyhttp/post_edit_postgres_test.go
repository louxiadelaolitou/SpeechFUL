package authprivacyhttp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"image"
	"image/color"
	"image/gif"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/authprivacy"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/postmedia"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
)

func TestRealHTTPSPostEditPostgres(t *testing.T) {
	ctx := context.Background()
	s := e2eNewServices(t)
	owner, _ := e2ePrivacySignup(t, s, "edit_https_owner")
	reader, readerID := e2ePrivacySignup(t, s, "edit_https_reader")
	channel := postHTTPSChannel(t, s, owner)
	identity := postHTTPSIdentity(t, s, owner, "原发言身份", 71)
	accepted := postHTTPSJSON(t, s, "PUT", "/api/v1/post-commands/"+httpTestEncoding(16, 72), owner, "", posts.CreatePostInput{
		ChannelID: channel, IdentityID: identity, Title: "原始标题", Body: "原始正文",
	}, 202)
	if _, err := s.c.PublishAcceptedPosts(ctx, 100); err != nil {
		t.Fatal(err)
	}
	id := accepted["postId"].(string)
	if _, err := s.cp.Exec(ctx, `INSERT INTO c_posts.post_likes(account_id,post_id) VALUES ($1,$2)`, readerID, id); err != nil {
		t.Fatal(err)
	}
	detail := "/api/v1/posts/" + id
	editor := "/api/v1/me/posts/" + id
	before := postHTTPSJSON(t, s, "GET", detail, reader, "", nil, 200)["post"].(map[string]any)
	caps := e2eJSON(t, s.client, "GET", s.cPublic.URL+editor+"/capabilities", nil, map[string]string{
		"Authorization": "Bearer " + owner, "X-Post-Edit": "v1",
	}, 200)
	if caps["canEdit"] != true || caps["canDelete"] != true {
		t.Fatal("author edit capability missing")
	}
	snapshot := postHTTPSJSON(t, s, "GET", editor+"/edit", owner, "", nil, 200)
	if !reflect.DeepEqual(before, snapshot["post"]) {
		t.Fatal("editor did not load current public post")
	}
	postHTTPSJSON(t, s, "GET", editor+"/edit", reader, "", nil, 404)
	base := snapshot["revision"].(string)
	first := editor + "/edit-commands/" + httpTestEncoding(16, 73)
	second := editor + "/edit-commands/" + httpTestEncoding(16, 74)
	invalid := editor + "/edit-commands/" + httpTestEncoding(16, 75)
	input := posts.EditInput{Revision: base, Title: "新标题", Body: "新正文"}
	postHTTPSJSON(t, s, "PUT", first, reader, "", input, 404)
	if got := postHTTPSJSON(t, s, "GET", first, owner, "", nil, 200); got["state"] != "PENDING" {
		t.Fatal("missing receipt falsely became terminal")
	}
	bad := input
	bad.Title = ""
	got := postHTTPSJSON(t, s, "PUT", invalid, owner, "", bad, 200)
	if got["code"] != "POST_CONTENT_INVALID" || !reflect.DeepEqual(got, postHTTPSJSON(t, s, "GET", invalid, owner, "", nil, 200)) {
		t.Fatal("failed edit has no authoritative receipt")
	}
	if got := postHTTPSJSON(t, s, "GET", detail, reader, "", nil, 200)["post"]; !reflect.DeepEqual(before, got) {
		t.Fatal("failed edit changed public content")
	}
	s.advanceC(time.Second)
	saved := postHTTPSJSON(t, s, "PUT", first, owner, "", input, 200)
	if saved["state"] != "SAVED" || saved["revision"] == base {
		t.Fatal("edit did not commit a new public version")
	}
	if got := postHTTPSJSON(t, s, "PUT", second, owner, "", input, 200); got["code"] != "POST_EDIT_CONFLICT" {
		t.Fatal("stale edit overwrote a newer one")
	}
	for _, method := range []string{"GET", "PUT"} {
		var body any
		if method == "PUT" {
			body = input
		}
		if !reflect.DeepEqual(saved, postHTTPSJSON(t, s, method, first, owner, "", body, 200)) {
			t.Fatal("edit replay did not return the original receipt")
		}
	}
	changed := input
	changed.Title = "不同请求"
	postHTTPSJSON(t, s, "PUT", first, owner, "", changed, 409)
	after := postHTTPSJSON(t, s, "GET", detail, reader, "", nil, 200)["post"].(map[string]any)
	if after["title"] != input.Title || after["body"] != input.Body {
		t.Fatal("reader did not receive saved content")
	}
	for _, field := range []string{"postId", "channelId", "author", "publishedAt"} {
		if !reflect.DeepEqual(after[field], before[field]) {
			t.Fatalf("edit changed %s", field)
		}
	}
	var likes int
	if err := s.cp.QueryRow(ctx, `SELECT count(*) FROM c_posts.post_likes WHERE account_id=$1 AND post_id=$2`, readerID, id).Scan(&likes); err != nil || likes != 1 {
		t.Fatal("edit changed the existing interaction")
	}
	request, err := http.NewRequest("GET", s.cPublic.URL+detail, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+reader)
	response, err := s.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if _, err := time.Parse(time.RFC3339Nano, response.Header.Get("Post-Edited-At")); err != nil {
		t.Fatal("edit time missing from public detail")
	}
	ownerToken, err := base64.RawURLEncoding.DecodeString(owner)
	if err != nil || len(ownerToken) != 32 {
		t.Fatal("bad synthetic author token")
	}
	type outcome struct {
		value authprivacy.PostResponse
		err   error
	}
	start := make(chan struct{})
	outcomes := make(chan outcome, 2)
	input.Revision = saved["revision"].(string)
	for i, key := range []byte{78, 79} {
		concurrent := input
		concurrent.Body = []string{"设备一保存", "设备二保存"}[i]
		go func() {
			<-start
			result, err := s.c.SavePostEdit(ctx, [32]byte(ownerToken), id, httpTestEncoding(16, key), concurrent)
			outcomes <- outcome{result, err}
		}()
	}
	close(start)
	var winningRevision string
	conflicts := 0
	for range 2 {
		result := <-outcomes
		if result.err != nil {
			t.Fatal(result.err)
		}
		edit := result.value.Value.(posts.EditResult)
		if edit.State == "SAVED" {
			winningRevision = edit.Revision
		} else if edit.Code == "POST_EDIT_CONFLICT" {
			conflicts++
		}
	}
	if winningRevision == "" || conflicts != 1 {
		t.Fatal("two concurrent writers did not serialize on the same revision")
	}
	postHTTPSJSON(t, s, "POST", detail+"/delete", owner, httpTestEncoding(16, 76), nil, 200)
	input.Revision = winningRevision
	late := editor + "/edit-commands/" + httpTestEncoding(16, 77)
	if got := postHTTPSJSON(t, s, "PUT", late, owner, "", input, 200); got["code"] != "POST_EDIT_DELETED" {
		t.Fatal("edit did not observe deletion")
	}
	postHTTPSJSON(t, s, "GET", detail, reader, "", nil, 404)
	if err := s.gate.Freeze(ctx, "synthetic edit verification freeze"); err != nil {
		t.Fatal(err)
	}
	postHTTPSJSON(t, s, "GET", first, owner, "", nil, 503)
}

func TestRealHTTPSPostEditMediaPostgres(t *testing.T) {
	ctx := context.Background()
	python := os.Getenv("POST_MEDIA_PYTHON")
	if python == "" {
		t.Skip("dedicated image validator absent")
	}
	script, err := filepath.Abs("../../media/validate_image.py")
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(script)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(source)
	validator, err := postmedia.NewProcessValidator(python, script, hex.EncodeToString(hash[:]))
	if err != nil {
		t.Fatal(err)
	}
	s := e2eNewServices(t, func(c *authprivacy.Community) *authprivacy.Community {
		enabled, err := c.WithPostImages(ctx, validator)
		if err == nil {
			enabled, err = enabled.WithModeration(ctx)
		}
		if err != nil {
			t.Fatal(err)
		}
		return enabled
	})
	owner, _ := e2ePrivacySignup(t, s, "edit_media_owner")
	reader, _ := e2ePrivacySignup(t, s, "edit_media_reader")
	reviewer, reviewerID := e2ePrivacySignup(t, s, "edit_media_reviewer")
	if _, err := s.cp.Exec(ctx, `INSERT INTO c_moderation.roles VALUES($1,'REVIEWER')`, reviewerID); err != nil {
		t.Fatal(err)
	}
	channel := postHTTPSChannel(t, s, owner)
	identity := postHTTPSIdentity(t, s, owner, "图片原作者", 81)
	reportIdentity := postHTTPSIdentity(t, s, reader, "图片举报人", 82)
	original := editTestGIF(t, 1)
	meta, err := validator.Inspect(ctx, original)
	if err != nil {
		t.Fatal(err)
	}
	item := postmedia.Selection{MediaID: uuid.NewString(), SHA256: meta.SHA256, ByteLength: meta.ByteLength, MIME: meta.MIME, Width: meta.Width, Height: meta.Height}
	created := postHTTPSJSON(t, s, "PUT", "/api/v1/post-commands/"+httpTestEncoding(16, 83), owner, "", map[string]any{
		"title": "编辑前", "body": "", "channelId": channel, "identityId": identity, "images": []postmedia.Selection{item},
	}, 202)
	id := created["postId"].(string)
	upload := "/api/v1/me/post-tasks/" + created["taskId"].(string) + "/attempts/1/images/" + item.MediaID
	grant := postHTTPSJSON(t, s, "POST", upload+"/upload-grant", owner, "", nil, 200)
	mediaHTTPSBytes(t, s, "PUT", upload+"/bytes", "Upload "+grant["uploadToken"].(string), meta.MIME, original, 204)
	if _, err := s.c.PublishAcceptedPosts(ctx, 100); err != nil {
		t.Fatal(err)
	}
	report := postHTTPSJSON(t, s, "PUT", "/api/v1/reports/"+uuid.NewString(), reader, "", map[string]any{
		"postId": id, "identityId": reportIdentity, "reason": "HARASSMENT", "description": "合成证据", "images": []any{},
	}, 200)
	caseID := report["report"].(map[string]any)["caseId"].(string)
	editor, detail := "/api/v1/me/posts/"+id, "/api/v1/posts/"+id
	snapshot := postHTTPSJSON(t, s, "GET", editor+"/edit", owner, "", nil, 200)
	originalID := snapshot["imageIds"].([]any)[0].(string)
	command := editor + "/edit-commands/" + httpTestEncoding(16, 84)
	newID, added := uuid.NewString(), editTestGIF(t, 0)
	addPath := detail + "/images/" + newID + "/original"
	stage := command + "/images/" + newID
	editStageHTTPS(t, s, stage, reader, added, 404)
	editStageHTTPS(t, s, stage, owner, added, 200)
	editStageHTTPS(t, s, stage, owner, added, 200)
	mediaHTTPSBytes(t, s, "GET", addPath, "Bearer "+reader, "", nil, 404)
	if got := postHTTPSJSON(t, s, "GET", detail, reader, "", nil, 200)["post"].(map[string]any); got["title"] != "编辑前" || got["imageCount"] != float64(1) {
		t.Fatal("staging changed public content")
	}
	base := snapshot["revision"].(string)
	missing := []string{uuid.NewString()}
	if got := postHTTPSJSON(t, s, "PUT", editor+"/edit-commands/"+httpTestEncoding(16, 85), owner, "", posts.EditInput{
		Revision: base, Title: "错误编辑", Body: "", ImageIDs: &missing,
	}, 200); got["code"] != "POST_EDIT_MEDIA_MISSING" {
		t.Fatal("incomplete media set was not rejected")
	}
	ids := []string{newID, originalID}
	input := posts.EditInput{Revision: base, Title: "编辑后", Body: "", ImageIDs: &ids}
	saved := postHTTPSJSON(t, s, "PUT", command, owner, "", input, 200)
	if saved["state"] != "SAVED" {
		t.Fatalf("image edit was not saved, code=%v", saved["code"])
	}
	current := postHTTPSJSON(t, s, "GET", detail+"/images", reader, "", nil, 200)["images"].([]any)
	if len(current) != 2 || current[0].(map[string]any)["contentVersionId"] != newID ||
		current[1].(map[string]any)["contentVersionId"] != originalID {
		t.Fatal("published image set was not switched in order")
	}
	if got := mediaHTTPSBytes(t, s, "GET", addPath, "Bearer "+reader, "", nil, 200); !bytes.Equal(got, added) {
		t.Fatal("edited image bytes differ")
	}
	ids = []string{newID}
	input = posts.EditInput{Revision: saved["revision"].(string), Title: "移除原图", Body: "新正文", ImageIDs: &ids}
	result := postHTTPSJSON(t, s, "PUT", editor+"/edit-commands/"+httpTestEncoding(16, 86), owner, "", input, 200)
	if result["state"] != "SAVED" {
		t.Fatal("remove edit failed")
	}
	mediaHTTPSBytes(t, s, "GET", detail+"/images/"+originalID+"/original", "Bearer "+reader, "", nil, 404)
	var evidenceTitle, evidenceImage string
	if err := s.cp.QueryRow(ctx, `SELECT evidence->>'title',evidence->'images'->0->>'base64' FROM c_moderation.cases WHERE case_id=$1`, caseID).Scan(&evidenceTitle, &evidenceImage); err != nil {
		t.Fatal(err)
	}
	if evidenceTitle != "编辑前" || evidenceImage != base64.StdEncoding.EncodeToString(original) {
		t.Fatal("submitted report evidence was overwritten")
	}
	postHTTPSJSON(t, s, "POST", "/api/v1/moderation/cases/"+caseID+"/decision", reviewer, "", map[string]any{
		"commandId": uuid.NewString(), "expectedVersion": 1, "action": "TAKEDOWN", "days": 0, "reason": "合成下架",
	}, 200)
	input.Revision = result["revision"].(string)
	if got := postHTTPSJSON(t, s, "PUT", editor+"/edit-commands/"+httpTestEncoding(16, 87), owner, "", input, 200); got["code"] != "POST_EDIT_HIDDEN" {
		t.Fatal("edit did not observe takedown")
	}
	mediaHTTPSBytes(t, s, "GET", addPath, "Bearer "+reader, "", nil, 404)
}

func editTestGIF(t *testing.T, index uint8) []byte {
	t.Helper()
	photo := image.NewPaletted(image.Rect(0, 0, 3, 2), color.Palette{color.Black, color.White})
	photo.SetColorIndex(0, 0, index)
	var buffer bytes.Buffer
	if err := gif.Encode(&buffer, photo, nil); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func editStageHTTPS(t *testing.T, s *e2eServices, path, token string, image []byte, status int) {
	t.Helper()
	request, err := http.NewRequest("PUT", s.cPublic.URL+path, bytes.NewReader(image))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "image/gif")
	response, err := s.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != status || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("edit image staging status=%d want=%d", response.StatusCode, status)
	}
}
