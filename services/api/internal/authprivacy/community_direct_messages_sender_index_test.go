package authprivacy

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// The sender-side pending count runs in every DM send transaction. dm_pending
// is recipient-first, so the count must be able to use 0054's partial index.
func TestDMSenderPendingIndexPostgres(t *testing.T) {
	l := newLab(t)
	ctx := context.Background()
	tx, e := l.cp.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SET LOCAL enable_seqscan=off; SET LOCAL enable_bitmapscan=off`); e != nil {
		t.Fatal(e)
	}
	rows, e := tx.Query(ctx, "EXPLAIN "+dmSenderPendingSQL, uuid.New())
	if e != nil {
		t.Fatal(e)
	}
	var plan strings.Builder
	for rows.Next() {
		var line string
		if e = rows.Scan(&line); e != nil {
			rows.Close()
			t.Fatal(e)
		}
		plan.WriteString(line)
		plan.WriteByte('\n')
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		t.Fatal(e)
	}
	if text := plan.String(); !strings.Contains(text, "dm_sender_pending") {
		t.Fatalf("sender pending count did not use its partial index:\n%s", text)
	}
}
