package storage

import (
	"context"
	"database/sql"
	"fmt"
	"iter"

	"github.com/wenooij/nuggit/api"
	"github.com/wenooij/nuggit/integrity"
	"github.com/wenooij/nuggit/pipes"
)

type PipeStore struct {
	db *sql.DB
}

func NewPipeStore(db *sql.DB) *PipeStore {
	return &PipeStore{db: db}
}

func (s *PipeStore) Delete(ctx context.Context, name integrity.NameDigest) error {
	return deleteSpec(ctx, s.db, "Pipes", name)
}

func (s *PipeStore) DeleteBatch(ctx context.Context, names []integrity.NameDigest) error {
	return deleteBatch(ctx, s.db, "Pipes", names)
}

func (s *PipeStore) Load(ctx context.Context, name integrity.NameDigest) (*api.Pipe, error) {
	c := new(api.Pipe)
	if err := loadSpec(ctx, s.db, "Pipes", name, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *PipeStore) LoadBatch(ctx context.Context, names []integrity.NameDigest) iter.Seq2[*api.Pipe, error] {
	return scanSpecsBatch(ctx, s.db, "Pipes", names, func() *api.Pipe { return new(api.Pipe) })
}

func (s *PipeStore) Store(ctx context.Context, pipe *api.Pipe) error {
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

	spec, err := marshalNullableJSONString(pipe.GetSpec())
	if err != nil {
		return err
	}

	// Disable old pipes by name.
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO ResourceLabels (ResourceID, Label)
SELECT r.ID, 'disabled'
FROM Resources AS r
JOIN Pipes AS p ON r.PipeID = p.ID
WHERE p.Name = ?`, pipe.GetName()); err != nil {
		return err
	}

	prep, err := tx.PrepareContext(ctx, "INSERT INTO Pipes (Name, Digest, TypeNumber, Spec) VALUES (?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer prep.Close()

	if _, err := prep.ExecContext(ctx,
		pipe.GetName(),
		pipe.GetDigest(),
		pipe.GetPoint().AsNumber(),
		spec,
	); err != nil {
		return handleExecErrors(err, alreadyExistsFunc("pipe", pipe))
	}

	if err := s.storePipeDeps(ctx, tx, pipe); err != nil {
		return err
	}

	// Update disabled on new pipe by name@digest.
	if _, err := tx.ExecContext(ctx, `DELETE FROM ResourceLabels WHERE ID IN (
	SELECT rl.ID
	FROM ResourceLabels AS rl
	JOIN Resources AS r ON rl.ResourceID = r.ID
	JOIN Pipes AS p ON r.PipeID = p.ID
	WHERE p.Name = ? AND p.Digest = ? AND rl.Label = 'disabled'
)`, pipe.GetName(), pipe.GetDigest()); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}

func (s *PipeStore) storePipeDeps(ctx context.Context, tx *sql.Tx, pipe *api.Pipe) error {
	prepDeps, err := tx.PrepareContext(ctx, `INSERT OR IGNORE INTO PipeDependencies (PipeID, ReferencedID)
	SELECT p.ID AS PipeID, p2.ID AS ReferencedID
	FROM Pipes AS p
	JOIN Pipes AS p2 ON 1
	WHERE p.Name = ? AND p.Digest = ? AND
		  p2.Name = ? AND p2.Digest = ? LIMIT 1`)
	if err != nil {
		return err
	}
	defer prepDeps.Close()

	for dep := range pipes.Deps(pipe.GetPipe()) {
		if _, err := prepDeps.ExecContext(ctx,
			pipe.GetName(),
			pipe.GetDigest(),
			dep.GetName(),
			dep.GetDigest(),
		); err != nil {
			return err
		}
	}

	return nil
}

func (s *PipeStore) StoreBatch(ctx context.Context, objects []*api.Pipe) error {
	// FIXME: Use a shared db connection for StoreBatch and Store.
	for _, o := range objects {
		err := s.Store(ctx, o)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *PipeStore) Scan(ctx context.Context) iter.Seq2[*api.Pipe, error] {
	return scanSpecs(ctx, s.db, "Pipes", func() *api.Pipe { return new(api.Pipe) })
}

func (s *PipeStore) ScanNames(ctx context.Context) iter.Seq2[integrity.NameDigest, error] {
	return scanNames(ctx, s.db, "Pipes")
}

func (s *PipeStore) ScanDependencies(ctx context.Context, pipe integrity.NameDigest) iter.Seq2[*api.Pipe, error] {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return seq2Error[*api.Pipe](err)
	}

	prep, err := conn.PrepareContext(ctx, `SELECT
    pr.Name AS ReferencedName,
    pr.Digest AS ReferencedDigest,
    pr.Spec
FROM PipeDependencies AS d
JOIN Pipes AS p ON d.PipeID = p.ID
JOIN Pipes AS pr ON d.ReferencedID = pr.ID
WHERE p.Name = ? AND p.Digest = ?`)
	if err != nil {
		return seq2Error[*api.Pipe](err)
	}

	queue := make([]integrity.NameDigest, 0, 16)
	queue = append(queue, integrity.Key(pipe))
	seen := make(map[integrity.NameDigest]struct{}, 16)

	return func(yield func(*api.Pipe, error) bool) {
		defer conn.Close()
		defer prep.Close()

		for len(queue) > 0 {
			pipe := queue[0]
			queue = queue[1:]

			rows, err := prep.QueryContext(ctx, pipe.GetName(), pipe.GetDigest())
			if err != nil {
				yield(nil, err)
				return
			}

			for rows.Next() {
				var name, digest, spec sql.NullString
				if err := rows.Scan(&name, &digest, &spec); err != nil {
					yield(nil, err)
					return
				}
				p := new(api.Pipe)
				if err := unmarshalNullableJSONString(spec, p); err != nil {
					yield(nil, err)
					return
				}
				if err := integrity.SetCheckNameDigest(p, name.String, digest.String); err != nil {
					yield(nil, fmt.Errorf("failed to set digest (%q): %w", name.String, err))
					return
				}
				if !yield(p, nil) {
					break
				}

				key := integrity.Key(p)
				if _, ok := seen[key]; !ok {
					queue = append(queue, integrity.Key(p))
					// Ignore cycles, but this is technically an invalid condition.
				}
				seen[key] = struct{}{}
			}
			if err := rows.Err(); err != nil {
				yield(nil, err)
			}
		}
	}
}
