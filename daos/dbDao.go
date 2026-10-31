package daos

import "context"

type DBDAO interface {
	Close() error
	GetInstances(ctx context.Context) ([]Instance, error)
	AddSaveRecord(ctx context.Context, instanceID int, filename string, size int64) error
	GetActiveSaves(ctx context.Context, instanceID int) ([]Save, error)
	MarkSaveDeleted(ctx context.Context, saveID int) error
}
