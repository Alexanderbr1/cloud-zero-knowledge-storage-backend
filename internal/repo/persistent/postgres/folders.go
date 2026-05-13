package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"cloud-backend/internal/entity"
	storageuc "cloud-backend/internal/usecase/storage"
)

const listFoldersRootSQL = `
	WITH RECURSIVE descendants(root_id, folder_id) AS (
		SELECT id AS root_id, id AS folder_id FROM folders WHERE user_id = $1 AND parent_id IS NULL AND deleted_at IS NULL
		UNION ALL
		SELECT d.root_id, f.id FROM folders f JOIN descendants d ON f.parent_id = d.folder_id WHERE f.deleted_at IS NULL
	),
	folder_sizes AS (
		SELECT d.root_id, COALESCE(SUM(b.file_size), 0) AS total_size
		FROM descendants d LEFT JOIN stored_blobs b ON b.folder_id = d.folder_id AND b.deleted_at IS NULL
		GROUP BY d.root_id
	)
	SELECT f.id, f.user_id, f.parent_id, f.name, f.created_at, COALESCE(fs.total_size, 0)
	FROM folders f LEFT JOIN folder_sizes fs ON fs.root_id = f.id
	WHERE f.user_id = $1 AND f.parent_id IS NULL AND f.deleted_at IS NULL
	ORDER BY f.name`

const listFoldersChildSQL = `
	WITH RECURSIVE descendants(root_id, folder_id) AS (
		SELECT id AS root_id, id AS folder_id FROM folders WHERE user_id = $1 AND parent_id = $2 AND deleted_at IS NULL
		UNION ALL
		SELECT d.root_id, f.id FROM folders f JOIN descendants d ON f.parent_id = d.folder_id WHERE f.deleted_at IS NULL
	),
	folder_sizes AS (
		SELECT d.root_id, COALESCE(SUM(b.file_size), 0) AS total_size
		FROM descendants d LEFT JOIN stored_blobs b ON b.folder_id = d.folder_id AND b.deleted_at IS NULL
		GROUP BY d.root_id
	)
	SELECT f.id, f.user_id, f.parent_id, f.name, f.created_at, COALESCE(fs.total_size, 0)
	FROM folders f LEFT JOIN folder_sizes fs ON fs.root_id = f.id
	WHERE f.user_id = $1 AND f.parent_id = $2 AND f.deleted_at IS NULL
	ORDER BY f.name`

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
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
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
		 FROM folders WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`,
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
		rows, err = s.pool.Query(ctx, listFoldersRootSQL, userID)
	} else {
		rows, err = s.pool.Query(ctx, listFoldersChildSQL, userID, parentID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFolders(rows)
}

func (s *Storage) RenameFolder(ctx context.Context, folderID, userID uuid.UUID, name string) (entity.Folder, error) {
	var f entity.Folder
	err := s.pool.QueryRow(ctx,
		`UPDATE folders SET name = $3
		 WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
		 RETURNING id, user_id, parent_id, name, created_at`,
		folderID, userID, name,
	).Scan(&f.ID, &f.UserID, &f.ParentID, &f.Name, &f.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.Folder{}, storageuc.ErrFolderNotFound
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
			return entity.Folder{}, storageuc.ErrFolderConflict
		}
		return entity.Folder{}, err
	}
	return f, nil
}

func (s *Storage) MoveFolder(ctx context.Context, p storageuc.MoveFolderParams) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE folders SET parent_id = $3
		 WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`,
		p.FolderID, p.UserID, p.NewParentID,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
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
			SELECT 1 FROM folders      WHERE parent_id = $1 AND deleted_at IS NULL
			UNION ALL
			SELECT 1 FROM stored_blobs WHERE folder_id = $1 AND deleted_at IS NULL
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

func (s *Storage) SearchFolders(ctx context.Context, userID uuid.UUID, query string) ([]entity.Folder, error) {
	rows, err := s.pool.Query(ctx, `
		WITH RECURSIVE descendants(root_id, folder_id) AS (
			SELECT id AS root_id, id AS folder_id FROM folders WHERE user_id = $1 AND name ILIKE $2
			UNION ALL
			SELECT d.root_id, f.id FROM folders f JOIN descendants d ON f.parent_id = d.folder_id
		),
		folder_sizes AS (
			SELECT d.root_id, COALESCE(SUM(b.file_size), 0) AS total_size
			FROM descendants d LEFT JOIN stored_blobs b ON b.folder_id = d.folder_id
			GROUP BY d.root_id
		)
		SELECT f.id, f.user_id, f.parent_id, f.name, f.created_at, COALESCE(fs.total_size, 0)
		FROM folders f LEFT JOIN folder_sizes fs ON fs.root_id = f.id
		WHERE f.user_id = $1 AND f.name ILIKE $2
		ORDER BY f.name
		LIMIT 50`,
		userID, "%"+query+"%",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFolders(rows)
}

func scanFolders(rows pgx.Rows) ([]entity.Folder, error) {
	var out []entity.Folder
	for rows.Next() {
		var f entity.Folder
		if err := rows.Scan(&f.ID, &f.UserID, &f.ParentID, &f.Name, &f.CreatedAt, &f.TotalSize); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
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
			WHERE s.depth < 100 AND f.deleted_at IS NULL
		)
		SELECT EXISTS (SELECT 1 FROM subtree WHERE id = $2)
	`, ancestorID, candidateID).Scan(&result)
	return result, err
}

// ─── Trash ────────────────────────────────────────────────────────────────────

// TrashFolder soft-deletes a folder and its entire subtree (subfolders + blobs) atomically.
func (s *Storage) TrashFolder(ctx context.Context, folderID, userID uuid.UUID) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Collect all folder IDs in the subtree (including root).
	rows, err := tx.Query(ctx, `
		WITH RECURSIVE subtree(id) AS (
			SELECT id FROM folders WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
			UNION ALL
			SELECT f.id FROM folders f JOIN subtree s ON f.parent_id = s.id WHERE f.deleted_at IS NULL
		)
		SELECT id FROM subtree
	`, folderID, userID)
	if err != nil {
		return false, err
	}
	ids, err := collectUUIDs(rows)
	if err != nil {
		return false, err
	}
	if len(ids) == 0 {
		return false, nil
	}

	if _, err := tx.Exec(ctx,
		`UPDATE folders SET deleted_at = NOW(), original_parent_id = parent_id WHERE id = ANY($1)`,
		ids,
	); err != nil {
		return false, err
	}

	if _, err := tx.Exec(ctx,
		`UPDATE stored_blobs
		 SET deleted_at = NOW(), original_folder_id = folder_id
		 WHERE folder_id = ANY($1) AND deleted_at IS NULL`,
		ids,
	); err != nil {
		return false, err
	}

	return true, tx.Commit(ctx)
}

// RestoreFolder restores a trashed folder and its entire subtree.
// The root folder is placed back under its original parent; if that parent is
// also trashed or gone, the folder is restored to the root level.
func (s *Storage) RestoreFolder(ctx context.Context, folderID, userID uuid.UUID) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var originalParentID *uuid.UUID
	err = tx.QueryRow(ctx,
		`SELECT original_parent_id FROM folders
		 WHERE id = $1 AND user_id = $2 AND deleted_at IS NOT NULL`,
		folderID, userID,
	).Scan(&originalParentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	// Collect all folder IDs in the subtree (traversing even trashed children via parent_id).
	rows, err := tx.Query(ctx, `
		WITH RECURSIVE subtree(id) AS (
			SELECT $1::uuid
			UNION ALL
			SELECT f.id FROM folders f JOIN subtree s ON f.parent_id = s.id
		)
		SELECT id FROM subtree
	`, folderID)
	if err != nil {
		return false, err
	}
	ids, err := collectUUIDs(rows)
	if err != nil {
		return false, err
	}

	// Verify the original parent still exists and is not trashed; fall back to root.
	restoredParentID := originalParentID
	if originalParentID != nil {
		var parentOK bool
		if err := tx.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM folders WHERE id = $1 AND deleted_at IS NULL)`,
			*originalParentID,
		).Scan(&parentOK); err != nil {
			return false, err
		}
		if !parentOK {
			restoredParentID = nil
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE folders
		SET deleted_at        = NULL,
		    original_parent_id = NULL,
		    parent_id          = CASE WHEN id = $1 THEN $3 ELSE parent_id END
		WHERE id = ANY($2)`,
		folderID, ids, restoredParentID,
	); err != nil {
		return false, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE stored_blobs
		SET deleted_at         = NULL,
		    folder_id          = COALESCE(original_folder_id, folder_id),
		    original_folder_id = NULL
		WHERE folder_id = ANY($1) AND deleted_at IS NOT NULL`,
		ids,
	); err != nil {
		return false, err
	}

	return true, tx.Commit(ctx)
}

// HardDeleteFolder permanently deletes a trashed folder subtree and returns
// the object keys of all contained blobs for MinIO cleanup.
func (s *Storage) HardDeleteFolder(ctx context.Context, folderID, userID uuid.UUID) (objectKeys []string, ok bool, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var owned bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM folders WHERE id = $1 AND user_id = $2 AND deleted_at IS NOT NULL)`,
		folderID, userID,
	).Scan(&owned); err != nil {
		return nil, false, err
	}
	if !owned {
		return nil, false, nil
	}

	rows, err := tx.Query(ctx, `
		WITH RECURSIVE subtree(id) AS (
			SELECT $1::uuid
			UNION ALL
			SELECT f.id FROM folders f JOIN subtree s ON f.parent_id = s.id
		)
		SELECT id FROM subtree
	`, folderID)
	if err != nil {
		return nil, false, err
	}
	ids, err := collectUUIDs(rows)
	if err != nil {
		return nil, false, err
	}

	blobRows, err := tx.Query(ctx,
		`DELETE FROM stored_blobs WHERE folder_id = ANY($1) RETURNING object_key`,
		ids,
	)
	if err != nil {
		return nil, false, err
	}
	objectKeys, err = scanObjectKeys(blobRows)
	if err != nil {
		return nil, false, err
	}

	// Break parent_id self-references within the set so the bulk DELETE succeeds.
	if _, err := tx.Exec(ctx,
		`UPDATE folders SET parent_id = NULL WHERE id = ANY($1)`, ids,
	); err != nil {
		return nil, false, err
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM folders WHERE id = ANY($1)`, ids,
	); err != nil {
		return nil, false, err
	}

	return objectKeys, true, tx.Commit(ctx)
}

func (s *Storage) ListTrashedFolders(ctx context.Context, userID uuid.UUID) ([]entity.Folder, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT f.id, f.user_id, f.parent_id, f.name, f.created_at, 0 AS total_size
		FROM folders f
		WHERE f.user_id = $1
		  AND f.deleted_at IS NOT NULL
		  AND (f.parent_id IS NULL OR NOT EXISTS(
		      SELECT 1 FROM folders p WHERE p.id = f.parent_id AND p.deleted_at IS NOT NULL
		  ))
		ORDER BY f.name`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFolders(rows)
}

func (s *Storage) EmptyTrashedFolders(ctx context.Context, userID uuid.UUID) ([]string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	blobRows, err := tx.Query(ctx, `
		DELETE FROM stored_blobs
		WHERE user_id = $1 AND folder_id IN (
		    SELECT id FROM folders WHERE user_id = $1 AND deleted_at IS NOT NULL
		)
		RETURNING object_key`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	objectKeys, err := scanObjectKeys(blobRows)
	if err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx,
		`UPDATE folders SET parent_id = NULL WHERE user_id = $1 AND deleted_at IS NOT NULL`, userID,
	); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM folders WHERE user_id = $1 AND deleted_at IS NOT NULL`, userID,
	); err != nil {
		return nil, err
	}

	return objectKeys, tx.Commit(ctx)
}

func (s *Storage) PurgeExpiredFolders(ctx context.Context, before time.Time) ([]string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	blobRows, err := tx.Query(ctx, `
		DELETE FROM stored_blobs
		WHERE folder_id IN (SELECT id FROM folders WHERE deleted_at IS NOT NULL AND deleted_at < $1)
		RETURNING object_key`,
		before,
	)
	if err != nil {
		return nil, err
	}
	objectKeys, err := scanObjectKeys(blobRows)
	if err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx,
		`UPDATE folders SET parent_id = NULL WHERE deleted_at IS NOT NULL AND deleted_at < $1`, before,
	); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM folders WHERE deleted_at IS NOT NULL AND deleted_at < $1`, before,
	); err != nil {
		return nil, err
	}

	return objectKeys, tx.Commit(ctx)
}

func collectUUIDs(rows pgx.Rows) ([]uuid.UUID, error) {
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
