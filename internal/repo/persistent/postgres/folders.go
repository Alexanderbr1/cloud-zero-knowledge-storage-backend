package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"cloud-backend/internal/entity"
	storageuc "cloud-backend/internal/usecase/storage"
)

var _ storageuc.FolderRegistry = (*Storage)(nil)

func (s *Storage) CreateFolder(ctx context.Context, p storageuc.CreateFolderParams) (entity.Folder, error) {
	var f entity.Folder
	err := s.pool.QueryRow(ctx,
		`INSERT INTO folders (user_id, parent_id, name)
		 VALUES ($1, $2, $3)
		 RETURNING id, user_id, parent_id, name, created_at`,
		p.UserID, p.ParentID, p.Name,
	).Scan(&f.ID, &f.UserID, &f.ParentID, &f.Name, &f.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return entity.Folder{}, storageuc.ErrFolderConflict
		}
		return entity.Folder{}, err
	}
	return f, nil
}

func (s *Storage) GetFolder(ctx context.Context, folderID, userID uuid.UUID) (entity.Folder, bool, error) {
	var f entity.Folder
	err := s.pool.QueryRow(ctx,
		`SELECT id, user_id, parent_id, name, created_at
		 FROM folders WHERE id = $1 AND user_id = $2`,
		folderID, userID,
	).Scan(&f.ID, &f.UserID, &f.ParentID, &f.Name, &f.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.Folder{}, false, nil
	}
	if err != nil {
		return entity.Folder{}, false, err
	}
	return f, true, nil
}

func (s *Storage) ListFolders(ctx context.Context, userID uuid.UUID, parentID *uuid.UUID) ([]entity.Folder, error) {
	var (
		rows pgx.Rows
		err  error
	)
	if parentID == nil {
		rows, err = s.pool.Query(ctx,
			`SELECT id, user_id, parent_id, name, created_at
			 FROM folders
			 WHERE user_id = $1 AND parent_id IS NULL
			 ORDER BY name`,
			userID,
		)
	} else {
		rows, err = s.pool.Query(ctx,
			`SELECT id, user_id, parent_id, name, created_at
			 FROM folders
			 WHERE user_id = $1 AND parent_id = $2
			 ORDER BY name`,
			userID, parentID,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []entity.Folder
	for rows.Next() {
		var f entity.Folder
		if err := rows.Scan(&f.ID, &f.UserID, &f.ParentID, &f.Name, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *Storage) RenameFolder(ctx context.Context, folderID, userID uuid.UUID, name string) (entity.Folder, error) {
	var f entity.Folder
	err := s.pool.QueryRow(ctx,
		`UPDATE folders SET name = $3
		 WHERE id = $1 AND user_id = $2
		 RETURNING id, user_id, parent_id, name, created_at`,
		folderID, userID, name,
	).Scan(&f.ID, &f.UserID, &f.ParentID, &f.Name, &f.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.Folder{}, storageuc.ErrFolderNotFound
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return entity.Folder{}, storageuc.ErrFolderConflict
		}
		return entity.Folder{}, err
	}
	return f, nil
}

func (s *Storage) MoveFolder(ctx context.Context, p storageuc.MoveFolderParams) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE folders SET parent_id = $3
		 WHERE id = $1 AND user_id = $2`,
		p.FolderID, p.UserID, p.NewParentID,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return storageuc.ErrFolderConflict
		}
		return err
	}
	if tag.RowsAffected() == 0 {
		return storageuc.ErrFolderNotFound
	}
	return nil
}

// DeleteFolder atomically checks that the folder is empty and deletes it within a
// single transaction, eliminating the TOCTOU race between the emptiness check and the
// DELETE. Returns ErrFolderNotEmpty if the folder has children (subfolders or blobs),
// ErrFolderNotFound if the row is absent or belongs to a different user.
func (s *Storage) DeleteFolder(ctx context.Context, folderID, userID uuid.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Verify ownership first — before the emptiness check — so that a non-owned
	// folderID always yields ErrFolderNotFound and never leaks whether the folder
	// is empty or not.
	var owned bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM folders WHERE id = $1 AND user_id = $2)`,
		folderID, userID,
	).Scan(&owned); err != nil {
		return err
	}
	if !owned {
		return storageuc.ErrFolderNotFound
	}

	var hasChildren bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM folders      WHERE parent_id = $1
			UNION ALL
			SELECT 1 FROM stored_blobs WHERE folder_id = $1
		)
	`, folderID).Scan(&hasChildren); err != nil {
		return err
	}
	if hasChildren {
		return storageuc.ErrFolderNotEmpty
	}

	if _, err := tx.Exec(ctx,
		`DELETE FROM folders WHERE id = $1 AND user_id = $2`,
		folderID, userID,
	); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// IsDescendantOf checks whether candidateID is in the subtree rooted at ancestorID.
// Used to prevent cycles when moving a folder into one of its own descendants.
func (s *Storage) IsDescendantOf(ctx context.Context, ancestorID, candidateID uuid.UUID) (bool, error) {
	var result bool
	err := s.pool.QueryRow(ctx, `
		WITH RECURSIVE subtree(id, depth) AS (
			SELECT id, 0 FROM folders WHERE id = $1
			UNION ALL
			SELECT f.id, s.depth + 1
			FROM folders f
			JOIN subtree s ON f.parent_id = s.id
			WHERE s.depth < 100
		)
		SELECT EXISTS (SELECT 1 FROM subtree WHERE id = $2)
	`, ancestorID, candidateID).Scan(&result)
	return result, err
}
