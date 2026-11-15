package authprivacy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/readtiming"
)

type observationTransaction struct {
	pgx.Tx
	terminalError error
	commits       int
	rollbacks     int
	child         pgx.Tx
}

func (tx *observationTransaction) Commit(context.Context) error {
	tx.commits++
	return tx.terminalError
}

func (tx *observationTransaction) Rollback(context.Context) error {
	tx.rollbacks++
	return tx.terminalError
}

func (tx *observationTransaction) Begin(context.Context) (pgx.Tx, error) {
	return tx.child, nil
}

func TestAuthorizationHoldObservationUsesTransactionBoundary(t *testing.T) {
	ctx, trace := readtiming.Start(context.Background())
	raw := &observationTransaction{child: &observationTransaction{}}
	tx := observeAuthorizationTransaction(ctx, raw)
	observeGateRowAcquired(ctx, tx)
	observeGateRowAcquired(ctx, tx)
	if trace.Snapshot().Stages["gate_hold"].Calls != 0 {
		t.Fatal("hold ended before transaction completion")
	}
	child, err := tx.Begin(ctx)
	if err != nil || child != raw.child {
		t.Fatal("observation changed savepoint delegation", err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	value := trace.Snapshot()
	if raw.commits != 1 || raw.rollbacks != 1 || value.Stages["gate_hold"].Calls != 1 || value.Stages["gate_hold_boundary_uncertain"].Calls != 0 {
		t.Fatalf("changed lifecycle or double-counted hold: %+v", value)
	}
}

func TestAuthorizationHoldObservationPreservesErrorsAndDisabledIdentity(t *testing.T) {
	background := context.Background()
	raw := &observationTransaction{terminalError: errors.New("unknown commit")}
	if observeAuthorizationTransaction(background, raw) != raw {
		t.Fatal("disabled timing wrapped the transaction")
	}
	ctx, trace := readtiming.Start(background)
	tx := observeAuthorizationTransaction(ctx, raw)
	observeGateRowAcquired(ctx, tx)
	if err := tx.Commit(ctx); err != raw.terminalError {
		t.Fatal("observation changed unknown commit result", err)
	}
	if trace.Snapshot().Stages["gate_hold_boundary_uncertain"].Calls != 1 {
		t.Fatal("unknown server lock-release boundary was not marked")
	}
	ctx, trace = readtiming.Start(background)
	tx = observeAuthorizationTransaction(ctx, &observationTransaction{})
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if trace.Snapshot().Stages["gate_hold"].Calls != 0 {
		t.Fatal("transaction without a successful Gate lock invented a hold")
	}
}

func TestGateHoldObservationExcludesLockWaitPostgres(t *testing.T) {
	control := newGateControl(t)
	ctx := context.Background()
	blocker, err := begin(ctx, control.lab.cp)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(ctx)
	row, err := control.gate.readGateRow(ctx, blocker)
	if err != nil {
		t.Fatal(err)
	}
	observed, trace := readtiming.Start(ctx)
	tx, err := begin(observed, control.lab.cp)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	ready := make(chan error, 1)
	go func() {
		_, err := control.gate.RecheckForCommit(observed, tx, uint64(row.generation))
		ready <- err
	}()
	deadline := time.Now().Add(3 * time.Second)
	waiting := false
	for time.Now().Before(deadline) {
		if err := control.lab.cp.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !waiting {
		t.Fatal("locking query never reached its deterministic blocker")
	}
	if trace.Snapshot().Stages["gate_hold"].Calls != 0 {
		t.Fatal("Gate hold was recorded while still waiting to acquire its row")
	}
	if err = blocker.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-ready:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Gate did not proceed after blocker committed")
	}
	if trace.Snapshot().Stages["gate_hold"].Calls != 0 {
		t.Fatal("Gate hold ended at recheck rather than transaction rollback")
	}
	if err = control.gate.Abort(observed, tx); err != nil {
		t.Fatal(err)
	}
	value := trace.Snapshot()
	if value.Stages["gate_hold"].Calls != 1 || value.Stages["gate_row_lock"].Calls != 1 || value.Stages["gate_hold_boundary_uncertain"].Calls != 0 {
		t.Fatalf("hold or separate abort transaction was counted incorrectly: %+v", value)
	}
}
