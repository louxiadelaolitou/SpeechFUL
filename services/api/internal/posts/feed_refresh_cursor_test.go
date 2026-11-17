package posts

import (
	"bytes"
	"testing"
	"time"
)

func TestFeedRefreshCursorBindings(t *testing.T) {
	c, e := NewCursorCodec(bytes.Repeat([]byte{9}, 32), "refresh-tests")
	if e != nil {
		t.Fatal(e)
	}
	at := time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)
	p := FeedRefreshPosition{AccountID: testID, ChannelID: otherID, Revision: 3, ExpiresAt: at.Add(CursorTTL), HeadAt: at, HeadOrdinal: 22, HeadID: testID, FloorAt: at.Add(-time.Hour), FloorOrdinal: 10, BeforeAt: at.Add(-time.Minute), BeforeOrdinal: 18, InsertAfter: otherID}
	token, e := c.EncodeFeedRefresh(p)
	if e != nil {
		t.Fatal(e)
	}
	got, e := c.DecodeFeedRefresh(token, p, at)
	if e != nil || got.HeadOrdinal != 22 || got.InsertAfter != otherID {
		t.Fatal(got, e)
	}
	for _, mutate := range []func(*FeedRefreshPosition){func(x *FeedRefreshPosition) { x.AccountID = otherID }, func(x *FeedRefreshPosition) { x.ChannelID = "" }, func(x *FeedRefreshPosition) { x.Revision++ }} {
		expected := p
		mutate(&expected)
		if _, e := c.DecodeFeedRefresh(token, expected, at); e == nil {
			t.Fatal("unbound refresh cursor")
		}
	}
	if _, e := c.DecodeFeedRefresh(token, p, p.ExpiresAt); e == nil {
		t.Fatal("expired token accepted")
	}
	regular := CursorPosition{Scope: CursorFeed, AccountID: testID, ChannelID: otherID, Limit: 5, Ceiling: 22, LastAt: at, LastOrdinal: 22, ExpiresAt: at.Add(CursorTTL), FilterRevision: 3}
	if _, e := c.Decode(token, regular, at); e == nil {
		t.Fatal("refresh token accepted for old direction")
	}
	old, e := c.Encode(regular)
	if e != nil {
		t.Fatal(e)
	}
	if _, e := c.DecodeFeedRefresh(old, p, at); e == nil {
		t.Fatal("old direction accepted for refresh")
	}
}
