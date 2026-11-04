package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net/http"

	"github.com/google/uuid"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
)

const editedProbeTitle = "回退后仍显示新版"
const editedProbeBody = "编辑后的正文保留"

func (p *probe) editPublishedImagePost(ctx context.Context, s *state) error {
	if s.Images == nil {
		return errors.New("post edit needs a published image fixture")
	}
	token := "Bearer " + s.Token
	headers := map[string]string{"Authorization": token}
	base := "/api/v1/me/posts/" + s.Images.PostID
	editor, _, err := p.request(ctx, "GET", p.c, base+"/edit", nil, headers, 200)
	if err != nil {
		return err
	}
	revision, err := field(editor, "revision")
	if err != nil {
		return err
	}
	key, err := randomCapability(16)
	if err != nil {
		return err
	}
	command := base + "/edit-commands/" + key
	replacement := uuid.NewString()
	picture := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	picture.SetNRGBA(0, 1, color.NRGBA{G: 220, A: 255})
	var encoded bytes.Buffer
	if err = png.Encode(&encoded, picture); err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, p.c+command+"/images/"+replacement, bytes.NewReader(encoded.Bytes()))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", token)
	request.Header.Set("Content-Type", "image/png")
	p.httpsRequests++
	response, err := p.https.Do(request)
	if err != nil {
		return errors.New("edit image upload transport failed")
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Cache-Control") != "no-store" {
		return errors.New("edit image staging failed")
	}
	ids := []string{replacement}
	result, _, err := p.request(ctx, "PUT", p.c, command, posts.EditInput{
		Revision: revision, Title: editedProbeTitle, Body: editedProbeBody, ImageIDs: &ids,
	}, headers, 200)
	if err != nil || result["state"] != "SAVED" {
		return errors.New("published image edit did not save")
	}
	sum := sha256.Sum256(encoded.Bytes())
	s.Images.MediaID = replacement
	s.Images.SHA256 = hex.EncodeToString(sum[:])
	s.Images.ByteLength = encoded.Len()
	return p.editedPostSnapshot(ctx, *s)
}

func (p *probe) editedPostSnapshot(ctx context.Context, s state) error {
	if s.Images == nil {
		return errors.New("edited image fixture absent")
	}
	body, headers, err := p.request(ctx, "GET", p.c, "/api/v1/posts/"+s.Images.PostID, nil, map[string]string{"Authorization": "Bearer " + s.Token}, 200)
	if err != nil {
		return err
	}
	post, ok := body["post"].(map[string]any)
	if !ok || post["title"] != editedProbeTitle || post["body"] != editedProbeBody || headers.Get("Post-Edited-At") == "" {
		return errors.New("edited title, body or time missing after program switch")
	}
	return p.imageSnapshot(ctx, s)
}
