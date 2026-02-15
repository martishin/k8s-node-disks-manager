package discover

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	v1alpha1 "github.com/martishin/k8s-node-disks-manager/api/v1alpha1"
	"github.com/martishin/k8s-node-disks-manager/internal/agent/store"
	"github.com/martishin/k8s-node-disks-manager/internal/shared"
)

type fakeStore struct {
	upserts []store.NodeDiskUpsert
	patches []statusPatchCall
}

type statusPatchCall struct {
	name string
	st   store.NodeStatusPatch
}

func (f *fakeStore) UpsertNodeDisk(_ context.Context, nd store.NodeDiskUpsert) error {
	f.upserts = append(f.upserts, nd)
	return nil
}

func (f *fakeStore) PatchNodeStatus(_ context.Context, name string, st store.NodeStatusPatch) error {
	f.patches = append(f.patches, statusPatchCall{name: name, st: st})
	return nil
}

func (f *fakeStore) WatchNodeDisks(_ context.Context, _ string) (<-chan store.NodeDiskEvent, error) {
	ch := make(chan store.NodeDiskEvent)
	close(ch)
	return ch, nil
}

func TestScanOnceDiscoversAndMarksReserved(t *testing.T) {
	tmpDir := t.TempDir()
	containerDir := filepath.Join(tmpDir, "container-disks")
	if err := os.MkdirAll(containerDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	diskPath := filepath.Join(containerDir, "disk1.img")
	if err := os.WriteFile(diskPath, make([]byte, 1024), 0o644); err != nil {
		t.Fatalf("write disk: %v", err)
	}
	if err := os.WriteFile(diskPath+".reserved", []byte("reserved\n"), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	fs := &fakeStore{}
	r := &Runner{
		NodeName:     "node-a",
		ContainerDir: containerDir,
		HostDir:      "/var/lib/node-disks-manager/disks",
		Store:        fs,
		defaultDesired: v1alpha1.NodeDiskDesiredSpec{
			State: v1alpha1.DesiredStateAvailable,
		},
	}

	if err := r.scanOnce(context.Background()); err != nil {
		t.Fatalf("scanOnce: %v", err)
	}

	if len(fs.upserts) != 1 {
		t.Fatalf("upserts=%d, want 1", len(fs.upserts))
	}
	up := fs.upserts[0]
	if up.Name != shared.NodeDiskName("node-a", "disk1") {
		t.Fatalf("unexpected name=%q", up.Name)
	}
	if up.Path != "/var/lib/node-disks-manager/disks/disk1.img" {
		t.Fatalf("unexpected path=%q", up.Path)
	}
	if up.DefaultTo.State != v1alpha1.DesiredStateAvailable {
		t.Fatalf("default desired=%q, want %q", up.DefaultTo.State, v1alpha1.DesiredStateAvailable)
	}

	if len(fs.patches) != 1 {
		t.Fatalf("patches=%d, want 1", len(fs.patches))
	}
	patch := fs.patches[0]
	if patch.st.Phase != v1alpha1.NodePhaseReserved {
		t.Fatalf("phase=%q, want %q", patch.st.Phase, v1alpha1.NodePhaseReserved)
	}
	if patch.st.CapacityBytes == nil {
		t.Fatal("capacity should be set")
	}
	if *patch.st.CapacityBytes != 1024 {
		t.Fatalf("capacity=%d, want 1024", *patch.st.CapacityBytes)
	}
	if patch.st.LastSeenTime.IsZero() {
		t.Fatal("lastSeenTime should be set")
	}
}

func TestScanOnceMarksMissingDisk(t *testing.T) {
	tmpDir := t.TempDir()
	containerDir := filepath.Join(tmpDir, "container-disks")
	if err := os.MkdirAll(containerDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	diskPath := filepath.Join(containerDir, "disk1.img")
	if err := os.WriteFile(diskPath, []byte("data"), 0o644); err != nil {
		t.Fatalf("write disk: %v", err)
	}

	fs := &fakeStore{}
	r := &Runner{
		NodeName:     "node-a",
		ContainerDir: containerDir,
		HostDir:      "/var/lib/node-disks-manager/disks",
		Store:        fs,
		defaultDesired: v1alpha1.NodeDiskDesiredSpec{
			State: v1alpha1.DesiredStateAvailable,
		},
	}

	if err := r.scanOnce(context.Background()); err != nil {
		t.Fatalf("first scanOnce: %v", err)
	}

	if err := os.Remove(diskPath); err != nil {
		t.Fatalf("remove disk: %v", err)
	}

	before := len(fs.patches)
	if err := r.scanOnce(context.Background()); err != nil {
		t.Fatalf("second scanOnce: %v", err)
	}

	if len(fs.patches) <= before {
		t.Fatalf("expected additional patch call; before=%d after=%d", before, len(fs.patches))
	}

	last := fs.patches[len(fs.patches)-1]
	if last.st.Phase != v1alpha1.NodePhaseMissing {
		t.Fatalf("phase=%q, want %q", last.st.Phase, v1alpha1.NodePhaseMissing)
	}
	if last.st.Message != "disk file no longer present" {
		t.Fatalf("message=%q", last.st.Message)
	}
	if last.st.CapacityBytes != nil {
		t.Fatalf("missing-disk patch should not set capacity, got %d", *last.st.CapacityBytes)
	}
	if time.Since(last.st.LastSeenTime) > 5*time.Second {
		t.Fatalf("lastSeenTime too old: %v", last.st.LastSeenTime)
	}
}
