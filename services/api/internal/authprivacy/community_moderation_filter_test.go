package authprivacy

import (
	"errors"
	"testing"

	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/moderation"
)

func TestModerationCaseFilter(t *testing.T) {
	state, number, e := moderationCaseFilter("PENDING", "42")
	if e != nil || state != "PENDING" || number != int64(42) {
		t.Fatalf("valid case filter rejected: state=%v number=%v err=%v", state, number, e)
	}
	state, number, e = moderationCaseFilter("", "")
	if e != nil || state != nil || number != nil {
		t.Fatalf("unfiltered case list changed: state=%v number=%v err=%v", state, number, e)
	}
	for _, test := range []struct {
		state  string
		number string
	}{
		{state: "OPEN"},
		{number: "0"},
		{number: "0042"},
		{number: "-1"},
		{number: "9223372036854775808"},
	} {
		if _, _, e = moderationCaseFilter(test.state, test.number); !errors.Is(e, moderation.ErrInvalid) {
			t.Fatalf("invalid case filter accepted: %+v err=%v", test, e)
		}
	}
}
