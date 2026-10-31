package daos

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

type SqliteDAO struct {
	db *sql.DB
}

func NewSqliteDAO(path string) (*SqliteDAO, error) {
	createTablesQuery := `CREATE TABLE IF NOT EXISTS instances (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		container_name varchar(255) NOT NULL UNIQUE,
		description text,
		dir_name text NOT NULL,
		keep_inventory boolean NOT NULL,
		s3_bucket VARCHAR(255) NOT NULL,
		prefix TEXT NOT NULL,
		working_path TEXT NOT NULL,
		active BOOLEAN DEFAULT TRUE NOT NULL,
		created_at BIGINT DEFAULT CURRENT_TIMESTAMP
	);
	
	CREATE TABLE IF NOT EXISTS saves (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		filename VARCHAR(255) NOT NULL,
		deleted BOOLEAN NOT NULL DEFAULT FALSE,
		size BIGINT NOT NULL,
		created_at BIGINT DEFAULT CURRENT_TIMESTAMP,
		instance_id INT NOT NULL,
		FOREIGN KEY (instance_id) REFERENCES instances(id)
	);`

	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("could not open DB: %w", err)
	}

	if err = db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("could not ping DB: %w", err)
	}

	if _, err = db.Exec(createTablesQuery); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("could not create tables: %w", err)
	}

	return &SqliteDAO{db: db}, nil
}

func (s *SqliteDAO) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

func (s *SqliteDAO) GetInstances(ctx context.Context) ([]Instance, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, container_name, description, dir_name, s3_bucket, prefix, working_path, active, keep_inventory FROM instances")
	if err != nil {
		return nil, fmt.Errorf("could not query instances: %w", err)
	}
	defer rows.Close()

	var instances []Instance
	for rows.Next() {
		var inst Instance
		if err := rows.Scan(&inst.ID, &inst.ContainerName, &inst.Description, &inst.DirName, &inst.S3Bucket, &inst.Prefix, &inst.WorkingPath, &inst.Active, &inst.KeepInventory); err != nil {
			return nil, fmt.Errorf("error scanning instance row: %w", err)
		}
		instances = append(instances, inst)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating instance rows: %w", err)
	}

	return instances, nil
}

func (s *SqliteDAO) AddSaveRecord(ctx context.Context, instanceID int, filename string, size int64) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO saves (filename, size, instance_id) VALUES (?, ?, ?)", filename, size, instanceID)
	if err != nil {
		return fmt.Errorf("could not insert save record: %w", err)
	}
	return nil
}

func (s *SqliteDAO) GetActiveSaves(ctx context.Context, instanceID int) ([]Save, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, filename, deleted, size, instance_id FROM saves WHERE deleted = 0 AND instance_id = ? ORDER BY created_at DESC", instanceID)
	if err != nil {
		return nil, fmt.Errorf("could not query active saves: %w", err)
	}
	defer rows.Close()

	var saves []Save
	for rows.Next() {
		var save Save
		if err := rows.Scan(&save.ID, &save.Filename, &save.Deleted, &save.Size, &save.InstanceID); err != nil {
			return nil, fmt.Errorf("error scanning save row: %w", err)
		}
		saves = append(saves, save)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating save rows: %w", err)
	}

	return saves, nil
}

func (s *SqliteDAO) MarkSaveDeleted(ctx context.Context, saveID int) error {
	_, err := s.db.ExecContext(ctx, "UPDATE saves SET deleted = 1 WHERE id = ?", saveID)
	if err != nil {
		return fmt.Errorf("could not mark save deleted: %w", err)
	}
	return nil
}
