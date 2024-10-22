package storage

import (
	"context"
	"database/sql"
	"iter"

	"github.com/wenooij/nuggit/api"
	"github.com/wenooij/nuggit/status"
)

type TriggerStore struct {
	db *sql.DB
}

func NewTriggerStore(db *sql.DB) *TriggerStore {
	return &TriggerStore{db: db}
}

func (s *TriggerStore) Delete(ctx context.Context, id string) error {
	return status.ErrUnimplemented
}

func (s *TriggerStore) Load(ctx context.Context, id string) (*api.TriggerRecord, error) {
	return nil, status.ErrUnimplemented
}

func (s *TriggerStore) Store(ctx context.Context, object *api.TriggerRecord) (string, error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	spec, err := marshalNullableJSONString(object.GetTrigger())
	if err != nil {
		return "", err
	}

	plan, err := marshalNullableJSONString(object.GetPlan())
	if err != nil {
		return "", err
	}

	prep, err := tx.PrepareContext(ctx, "INSERT INTO Triggers (TriggerID, Committed, Plan, Spec) VALUES (?, false, ?, ?)")
	if err != nil {
		return "", err
	}

	id, err := newUUID()
	if err != nil {
		return "", err
	}

	if _, err := prep.ExecContext(ctx,
		id,
		spec,
		plan,
	); err != nil {
		return "", err
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}

	return id, nil
}

func (s *TriggerStore) Scan(ctx context.Context, scanFn func(object *api.Trigger, err error) error) error {
	return status.ErrUnimplemented
}

func (s *TriggerStore) StoreTriggerCollections(ctx context.Context, trigger string, collections []api.NameDigest) error {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, c := range collections {
		prep, err := tx.PrepareContext(ctx, `INSERT INTO TriggerCollections (TriggerID, CollectionName, CollectionDigest) VALUES (?, ?, ?)`)
		if err != nil {
			return err
		}
		defer prep.Close()

		if _, err := prep.ExecContext(ctx,
			trigger,
			c.Name,
			c.Digest,
		); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}

func (s *TriggerStore) ScanTriggerCollections(ctx context.Context, trigger string) iter.Seq2[*api.Collection, error] {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return seq2Error[*api.Collection](err)
	}

	prep, err := conn.PrepareContext(ctx, `SELECT
	t.CollectionName,
	t.CollectionDigest,
	c.Spec
FROM TriggerCollections AS t
JOIN Collections AS c ON t.CollectionName = c.Name AND t.CollectionDigest = c.Digest
WHERE t.TriggerID = ?`)
	if err != nil {
		return seq2Error[*api.Collection](err)
	}

	rows, err := prep.QueryContext(ctx, trigger)
	if err != nil {
		return seq2Error[*api.Collection](err)
	}

	return func(yield func(*api.Collection, error) bool) {
		defer conn.Close()
		defer rows.Close()

		for rows.Next() {
			var name, digest, spec sql.NullString
			if err := rows.Scan(&name, &digest, &spec); err != nil {
				yield(nil, err)
				return
			}
			c := new(api.Collection)
			if err := unmarshalNullableJSONString(spec, c); err != nil {
				yield(nil, err)
				return
			}
			c.SetNameDigest(api.NameDigest{Name: name.String, Digest: digest.String})
			if !yield(c, nil) {
				break
			}
		}
		if err := rows.Err(); err != nil {
			yield(nil, err)
		}
	}
}
