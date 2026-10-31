package daos

import (
	"path/filepath"
	"testing"
)

func TestSqliteDAO(t *testing.T) {
	ctx := t.Context()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	dao, err := NewSqliteDAO(dbPath)
	if err != nil {
		t.Fatalf("failed to create sqlite dao: %v", err)
	}
	defer dao.Close()

	// Initially empty instances
	instances, err := dao.GetInstances(ctx)
	if err != nil {
		t.Fatalf("failed to get instances: %v", err)
	}
	if len(instances) != 0 {
		t.Fatalf("expected 0 instances, got %d", len(instances))
	}

	// Insert instance manually for test
	_, err = dao.db.ExecContext(ctx, `INSERT INTO instances 
		(container_name, description, dir_name, keep_inventory, s3_bucket, prefix, working_path, active) 
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"mc-server-1", "Test Server", "world", true, "my-bucket", "backups", "/mc/data", true,
	)
	if err != nil {
		t.Fatalf("failed to insert test instance: %v", err)
	}

	instances, err = dao.GetInstances(ctx)
	if err != nil {
		t.Fatalf("failed to get instances: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(instances))
	}
	inst := instances[0]
	if inst.ContainerName != "mc-server-1" || !inst.Active || !inst.KeepInventory || inst.DirName != "world" {
		t.Errorf("unexpected instance data: %+v", inst)
	}

	// Add saves
	err = dao.AddSaveRecord(ctx, inst.ID, "world_save1.tar.gz", 1024)
	if err != nil {
		t.Fatalf("failed to add save record 1: %v", err)
	}
	err = dao.AddSaveRecord(ctx, inst.ID, "world_save2.tar.gz", 2048)
	if err != nil {
		t.Fatalf("failed to add save record 2: %v", err)
	}

	saves, err := dao.GetActiveSaves(ctx, inst.ID)
	if err != nil {
		t.Fatalf("failed to get active saves: %v", err)
	}
	if len(saves) != 2 {
		t.Fatalf("expected 2 active saves, got %d", len(saves))
	}

	// Mark first save as deleted
	saveToDel := saves[0]
	err = dao.MarkSaveDeleted(ctx, saveToDel.ID)
	if err != nil {
		t.Fatalf("failed to mark save deleted: %v", err)
	}

	savesAfter, err := dao.GetActiveSaves(ctx, inst.ID)
	if err != nil {
		t.Fatalf("failed to get active saves after deletion: %v", err)
	}
	if len(savesAfter) != 1 {
		t.Fatalf("expected 1 active save, got %d", len(savesAfter))
	}
	if savesAfter[0].ID == saveToDel.ID {
		t.Errorf("deleted save was still returned")
	}
}
