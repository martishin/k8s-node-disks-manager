package shared

import (
	"strings"
	"testing"
)

func TestNodeDiskNameNormalization(t *testing.T) {
	got := NodeDiskName("Node_A.01", "Disk_1.DATA")
	want := "node-disk-node-a-01-disk-1-data"
	if got != want {
		t.Fatalf("NodeDiskName()=%q, want %q", got, want)
	}
}

func TestNodeDiskNameLengthBound(t *testing.T) {
	node := strings.Repeat("n", 400)
	disk := strings.Repeat("d", 400)
	got := NodeDiskName(node, disk)
	if len(got) > 253 {
		t.Fatalf("name length=%d > 253", len(got))
	}
}
