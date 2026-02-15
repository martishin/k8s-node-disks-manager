package reconcile

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	v1alpha1 "github.com/martishin/k8s-node-disks-manager/api/v1alpha1"
	"github.com/martishin/k8s-node-disks-manager/internal/agent/store"
)

type fakeStore struct {
	patches []statusPatchCall
}

type statusPatchCall struct {
	name string
	st   store.NodeStatusPatch
}

func (f *fakeStore) UpsertNodeDisk(_ context.Context, _ store.NodeDiskUpsert) error {
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

func TestApplyDesiredReserveThenRelease(t *testing.T) {
	tmpMount := t.TempDir()
	diskPath := filepath.Join(tmpMount, "disks", "disk1.img")
	if err := os.MkdirAll(filepath.Dir(diskPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(diskPath, []byte("disk"), 0o644); err != nil {
		t.Fatalf("write disk: %v", err)
	}

	fs := &fakeStore{}
	r := &Runner{
		NodeName:      "node-a",
		HostRootPath:  "/var/lib/node-disks-manager",
		MountRootPath: tmpMount,
		Store:         fs,
	}

	reserveEvt := store.NodeDiskEvent{
		Name:         "node-disk-node-a-disk1",
		NodeName:     "node-a",
		DiskID:       "disk1",
		Path:         "/var/lib/node-disks-manager/disks/disk1.img",
		DesiredState: v1alpha1.DesiredStateReserved,
	}

	if err := r.applyDesired(context.Background(), reserveEvt); err != nil {
		t.Fatalf("reserve applyDesired: %v", err)
	}
	marker := diskPath + ".reserved"
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("marker should exist: %v", err)
	}
	if len(fs.patches) != 1 || fs.patches[0].st.Phase != v1alpha1.NodePhaseReserved {
		t.Fatalf("unexpected reserve patch: %+v", fs.patches)
	}

	releaseEvt := reserveEvt
	releaseEvt.DesiredState = v1alpha1.DesiredStateAvailable
	if err := r.applyDesired(context.Background(), releaseEvt); err != nil {
		t.Fatalf("release applyDesired: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("marker should be removed, stat err=%v", err)
	}
	if len(fs.patches) != 2 || fs.patches[1].st.Phase != v1alpha1.NodePhaseAvailable {
		t.Fatalf("unexpected release patch: %+v", fs.patches)
	}
}

func TestToContainerPath(t *testing.T) {
	got := toContainerPath("/var/lib/node-disks-manager/disks/disk1.img", "/var/lib/node-disks-manager", "/host-node-disks-manager")
	want := filepath.Join("/host-node-disks-manager", "disks", "disk1.img")
	if got != want {
		t.Fatalf("toContainerPath()=%q, want %q", got, want)
	}
}

func TestRunIgnoresDeletedEvent(t *testing.T) {
	fs := &fakeStore{}
	r := &Runner{
		NodeName:      "node-a",
		HostRootPath:  "/var/lib/node-disks-manager",
		MountRootPath: t.TempDir(),
		Store:         fs,
	}

	events := make(chan store.NodeDiskEvent, 1)
	events <- store.NodeDiskEvent{
		Type:         store.NodeDiskEventDeleted,
		Name:         "node-disk-node-a-disk1",
		NodeName:     "node-a",
		DiskID:       "disk1",
		Path:         "/var/lib/node-disks-manager/disks/disk1.img",
		DesiredState: v1alpha1.DesiredStateReserved,
	}
	close(events)

	if err := r.Run(context.Background(), events); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(fs.patches) != 0 {
		t.Fatalf("expected no status patches for deleted events, got %d", len(fs.patches))
	}

	// Ensure no delayed async writes happened.
	time.Sleep(10 * time.Millisecond)
	if len(fs.patches) != 0 {
		t.Fatalf("expected no delayed status patches for deleted events, got %d", len(fs.patches))
	}
}
