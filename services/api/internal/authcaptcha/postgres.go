package authcaptcha

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SQLStore struct {
	pool   *pgxpool.Pool
	schema string
}

func NewSQL(pool *pgxpool.Pool, schema string) *Service {
	if schema != "c_auth" && schema != "v_auth" {
		panic("invalid captcha schema")
	}
	return New(&SQLStore{pool: pool, schema: schema})
}

func (s *SQLStore) Issue(ctx context.Context, id [32]byte, r Record) error {
	// Indexed, bounded expiry cleanup; never touches account or mail records.
	for _, table := range []string{"captcha_challenges", "captcha_proofs"} {
		_, err := s.pool.Exec(ctx, "DELETE FROM "+s.schema+"."+table+" WHERE id IN (SELECT id FROM "+s.schema+"."+table+" WHERE expires_at<=clock_timestamp() LIMIT 256)")
		if err != nil {
			return err
		}
	}
	_, err := s.pool.Exec(ctx, "INSERT INTO "+s.schema+".captcha_challenges(id,binding,answer_x,expires_at) VALUES($1,$2,$3,clock_timestamp()+interval '120 seconds')", id[:], r.Binding[:], r.X)
	return err
}
func (s *SQLStore) Take(ctx context.Context, id [32]byte) (Record, error) {
	var r Record
	var binding []byte
	var valid bool
	err := s.pool.QueryRow(ctx, "DELETE FROM "+s.schema+".captcha_challenges WHERE id=$1 RETURNING binding,answer_x,expires_at,expires_at>clock_timestamp()", id[:]).Scan(&binding, &r.X, &r.Expires, &valid)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !valid {
		err = ErrInvalid
	}
	copy(r.Binding[:], binding)
	return r, err
}
func (s *SQLStore) Grant(ctx context.Context, id, binding [32]byte) error {
	_, err := s.pool.Exec(ctx, "INSERT INTO "+s.schema+".captcha_proofs(id,binding,expires_at) VALUES($1,$2,clock_timestamp()+interval '60 seconds')", id[:], binding[:])
	return err
}
func (s *SQLStore) Consume(ctx context.Context, id, binding [32]byte) error {
	var valid bool
	err := s.pool.QueryRow(ctx, "DELETE FROM "+s.schema+".captcha_proofs WHERE id=$1 RETURNING binding=$2 AND expires_at>clock_timestamp()", id[:], binding[:]).Scan(&valid)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !valid {
		return ErrInvalid
	}
	return err
}
