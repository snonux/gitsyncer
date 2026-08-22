package sync

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/snonux/gitsyncer/internal/config"
)

func TestSyncRepository_DryRunDoesNotCreateNonexistentWorkDir(t *testing.T) {
	workDir := filepath.Join(t.TempDir(), "missing", "work")
	syncer := New(&config.Config{Organizations: []config.Organization{{Host: "git@example.invalid", Name: "owner"}}}, workDir)
	syncer.SetDryRun(true)

	if err := syncer.SyncRepository("demo"); err != nil {
		t.Fatalf("SyncRepository() error = %v", err)
	}
	if _, err := os.Stat(workDir); !os.IsNotExist(err) {
		t.Fatalf("dry run created or changed nonexistent work directory: stat error = %v", err)
	}
}

func TestSyncRepository_DryRunLeavesExistingWorkDirUnchanged(t *testing.T) {
	workDir := t.TempDir()
	repoPath := filepath.Join(workDir, "demo")
	if output, err := exec.Command("git", "init", repoPath).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, output)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "dirty.txt"), []byte("uncommitted\n"), 0644); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, workDir)

	syncer := New(&config.Config{Organizations: []config.Organization{{Host: "git@example.invalid", Name: "owner"}}}, workDir)
	syncer.SetDryRun(true)
	if err := syncer.SyncRepository("demo"); err != nil {
		t.Fatalf("SyncRepository() error = %v", err)
	}

	after := snapshotTree(t, workDir)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("dry run changed work directory\nbefore: %#v\nafter:  %#v", before, after)
	}
}

func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	snapshot := make(map[string]string)
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		value := fmt.Sprintf("%s:%s", info.Mode(), entry.Type())
		if info.Mode().IsRegular() {
			contents, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			value += fmt.Sprintf(":%x", sha256.Sum256(contents))
		}
		snapshot[rel] = value
		return nil
	}); err != nil {
		t.Fatalf("snapshot %s: %v", root, err)
	}
	return snapshot
}
