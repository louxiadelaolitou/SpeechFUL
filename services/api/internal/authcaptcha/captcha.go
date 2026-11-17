// Package authcaptcha supplies locally hosted, single-use slide challenges.
// It does not authenticate accounts or replace existing admission/Safety Gates.
package authcaptcha

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"math"
	"time"

	"github.com/wenlng/go-captcha/v2/base/option"
	"github.com/wenlng/go-captcha/v2/slide"
)

var ErrInvalid = errors.New("captcha invalid or expired")
var ErrBusy = errors.New("captcha generation busy")

type Record struct {
	Binding [32]byte
	X       int
	Expires time.Time
}

type Store interface {
	Issue(context.Context, [32]byte, Record) error
	Take(context.Context, [32]byte) (Record, error)
	Grant(context.Context, [32]byte, [32]byte) error
	Consume(context.Context, [32]byte, [32]byte) error
}

type Service struct {
	store Store
	slots chan struct{}
}

func New(store Store) *Service { return &Service{store: store, slots: make(chan struct{}, 4)} }

// Binding contains only identifiers already owned by the receiving C or V.
// Different purposes, installations, subjects and original keys never share proof.
func Binding(action, installation, key, subject string) [32]byte {
	return sha256.Sum256([]byte("SPEECHFUL/CAPTCHA/V1\x00" + action + "\x00" + installation + "\x00" + key + "\x00" + subject))
}

type Challenge struct {
	ID          string    `json:"id"`
	Background  string    `json:"background"`
	Piece       string    `json:"piece"`
	Width       int       `json:"width"`
	Height      int       `json:"height"`
	PieceWidth  int       `json:"pieceWidth"`
	PieceHeight int       `json:"pieceHeight"`
	StartX      int       `json:"startX"`
	Y           int       `json:"y"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

func randomToken() (string, [32]byte, error) {
	var raw [32]byte
	_, err := rand.Read(raw[:])
	return base64.RawURLEncoding.EncodeToString(raw[:]), sha256.Sum256(raw[:]), err
}

func digest(token string) ([32]byte, error) {
	raw, err := base64.RawURLEncoding.Strict().DecodeString(token)
	if err != nil || len(raw) != 32 {
		return [32]byte{}, ErrInvalid
	}
	return sha256.Sum256(raw), nil
}

func (s *Service) Create(ctx context.Context, binding [32]byte) (Challenge, error) {
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		return Challenge{}, ErrBusy
	}
	// Original geometric artwork, generated per challenge; no external image calls.
	var seed [256]byte
	if _, err := rand.Read(seed[:]); err != nil {
		return Challenge{}, err
	}
	bg := image.NewRGBA(image.Rect(0, 0, 320, 160))
	for y := 0; y < 160; y++ {
		for x := 0; x < 320; x++ {
			v := float64(x)/320 + float64(y)/240
			bg.SetRGBA(x, y, color.RGBA{uint8(35 + 35*v), uint8(105 + 35*v), uint8(140 + 35*v), 255})
		}
	}
	for i := 0; i < 32; i++ {
		cx, cy, r := int(seed[i*4])*320/256, int(seed[i*4+1])*160/256, 12+int(seed[i*4+2])%33
		c := color.RGBA{seed[i*4+3]/2 + 80, seed[i*4]/2 + 100, seed[i*4+1]/2 + 100, 255}
		for y := max(0, cy-r); y < min(160, cy+r); y++ {
			for x := max(0, cx-r); x < min(320, cx+r); x++ {
				if (x-cx)*(x-cx)+(y-cy)*(y-cy) < r*r {
					bg.SetRGBA(x, y, c)
				}
			}
		}
	}
	builder := slide.NewBuilder(slide.WithImageSize(option.Size{Width: 320, Height: 160}), slide.WithRangeGraphSize(option.RangeVal{Min: 44, Max: 52}))
	builder.SetResources(slide.WithBackgrounds([]image.Image{bg}), slide.WithGraphImages([]*slide.GraphImage{puzzleGraph()}))
	data, err := builder.Make().Generate()
	if err != nil {
		return Challenge{}, err
	}
	block := data.GetData()
	background, err := data.GetMasterImage().ToBase64()
	if err != nil {
		return Challenge{}, err
	}
	piece, err := data.GetTileImage().ToBase64()
	if err != nil {
		return Challenge{}, err
	}
	id, hash, err := randomToken()
	if err != nil {
		return Challenge{}, err
	}
	expires := time.Now().UTC().Add(2 * time.Minute)
	err = s.store.Issue(ctx, hash, Record{Binding: binding, X: block.X, Expires: expires})
	return Challenge{ID: id, Background: background, Piece: piece, Width: 320, Height: 160, PieceWidth: block.Width, PieceHeight: block.Height, StartX: block.DX, Y: block.DY, ExpiresAt: expires}, err
}

// Original puzzle template, kept in source so deployment needs no image files.
func puzzleGraph() *slide.GraphImage {
	mask := image.NewRGBA(image.Rect(0, 0, 64, 64))
	shadow := image.NewRGBA(mask.Bounds())
	overlay := image.NewRGBA(mask.Bounds())
	inside := func(x, y int) bool {
		body := x >= 9 && x < 55 && y >= 12 && y < 58
		top := (x-32)*(x-32)+(y-12)*(y-12) < 9*9
		notch := (x-9)*(x-9)+(y-35)*(y-35) < 8*8
		return (body || top) && !notch
	}
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			if inside(x, y) {
				mask.SetRGBA(x, y, color.RGBA{255, 255, 255, 255})
				shadow.SetRGBA(x, y, color.RGBA{10, 20, 30, 175})
				if !inside(x-1, y) || !inside(x+1, y) || !inside(x, y-1) || !inside(x, y+1) {
					overlay.SetRGBA(x, y, color.RGBA{255, 255, 255, 230})
				}
			}
		}
	}
	return &slide.GraphImage{OverlayImage: overlay, ShadowImage: shadow, MaskImage: mask}
}

func (s *Service) Solve(ctx context.Context, id string, x float64) (string, error) {
	hash, err := digest(id)
	if err != nil {
		return "", err
	}
	record, err := s.store.Take(ctx, hash) // One attempt, including wrong answers.
	if err != nil {
		return "", err
	}
	if !time.Now().Before(record.Expires) || math.IsNaN(x) || math.IsInf(x, 0) || math.Abs(x-float64(record.X)) > 4 {
		return "", ErrInvalid
	}
	token, hash, err := randomToken()
	if err == nil {
		err = s.store.Grant(ctx, hash, record.Binding)
	}
	return token, err
}

func (s *Service) Verify(ctx context.Context, token string, binding [32]byte) error {
	hash, err := digest(token)
	if err != nil {
		return err
	}
	return s.store.Consume(ctx, hash, binding)
}
