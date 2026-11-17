package authprivacy

import (
	"bytes"
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/readtiming"
)

// Batch preserves three distinct statements and their server execution order.
// Only already-bound native prepared reads use it. Every command is a SELECT
// for that same caller, in the existing explicit final transaction, before
// the Gate lock. No dependent authorization decision is pipelined or cached.
func lockPreparedPostSession(ctx context.Context, tx pgx.Tx, account uuid.UUID, token [32]byte) (a credentialAccount, session credentialSession, mute restriction, err error) {
	a.account, session.token = account, token
	batch := &pgx.Batch{}
	batch.Queue(`SELECT state,session_generation FROM c_auth.accounts WHERE account_id=$1 FOR UPDATE`, account)
	batch.Queue(`SELECT ban_state,ban_ends_at,mute_state,mute_ends_at FROM c_auth.account_restrictions WHERE account_id=$1 FOR UPDATE`, account)
	batch.Queue(lockCredentialSessionSQL, account, token[:])
	results := tx.SendBatch(ctx, batch)
	defer func() {
		if closeErr := results.Close(); closeErr != nil {
			err = ErrAuthorizationUnavailable
		}
	}()
	if err = results.QueryRow().Scan(&a.state, &a.generation); err != nil {
		return a, session, mute, ErrAuthorizationUnavailable
	}
	if err = results.QueryRow().Scan(&a.ban.state, &a.ban.ends, &mute.state, &mute.ends); err != nil {
		return a, session, mute, ErrAuthorizationUnavailable
	}
	err = results.QueryRow().Scan(&session.generation, &session.authGeneration, &session.expires, &session.lastActivity, &session.revoked, &session.reason)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrSessionInvalid
	} else if err != nil {
		err = ErrAuthorizationUnavailable
	}
	return
}

// The proof and display counters remain two fresh SQL statements. They are
// sent together only after final account/session/Gate checks. A failed proof,
// scan, close or later cursor check discards the counts with the entire plan.
func (p *feedReadProof) revalidateWithViewCounts(ctx context.Context, tx pgx.Tx, account uuid.UUID, ids []string) (counts map[string]int64, err error) {
	batch := &pgx.Batch{}
	batch.Queue(feedReadVersionsSQL, account, p.ids, nullablePostID(p.channel), p.initial)
	batch.Queue(preparedViewCountsSQL, ids)
	finish := readtiming.Span(ctx, "list_proof")
	results := tx.SendBatch(ctx, batch)
	defer func() {
		if closeErr := results.Close(); closeErr != nil {
			err = closeErr
		}
		if err != nil {
			counts = nil
		}
	}()
	current, e := scanFeedReadProof(results.QueryRow())
	finish()
	if e != nil {
		return nil, e
	}
	if !bytes.Equal(current, p.versions) {
		return nil, errReadConflict
	}
	rows, e := results.Query()
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	counts = make(map[string]int64, len(ids))
	for rows.Next() {
		var id string
		var count int64
		if e = rows.Scan(&id, &count); e != nil {
			return nil, e
		}
		counts[id] = count
	}
	return counts, rows.Err()
}
