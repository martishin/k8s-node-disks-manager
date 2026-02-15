package store

import (
	"testing"

	v1alpha1 "github.com/martishin/k8s-node-disks-manager/api/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/watch"
)

func TestObjectToEventMapsFieldsAndType(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name": "node-disk-node-a-disk1",
		},
		"spec": map[string]any{
			"nodeName": "node-a",
			"diskID":   "disk1",
			"path":     "/var/lib/node-disks-manager/disks/disk1.img",
			"desired": map[string]any{
				"state": "Reserved",
				"owner": "auto",
			},
		},
	}}

	evt, ok := objectToEvent(obj, NodeDiskEventAdded)
	if !ok {
		t.Fatal("objectToEvent() should succeed")
	}

	if evt.Type != NodeDiskEventAdded {
		t.Fatalf("Type=%q, want %q", evt.Type, NodeDiskEventAdded)
	}
	if evt.Name != "node-disk-node-a-disk1" {
		t.Fatalf("Name=%q", evt.Name)
	}
	if evt.NodeName != "node-a" || evt.DiskID != "disk1" {
		t.Fatalf("unexpected identity fields: %+v", evt)
	}
	if evt.DesiredState != v1alpha1.DesiredStateReserved {
		t.Fatalf("DesiredState=%q, want %q", evt.DesiredState, v1alpha1.DesiredStateReserved)
	}
	if evt.DesiredOwner != "auto" {
		t.Fatalf("DesiredOwner=%q, want auto", evt.DesiredOwner)
	}
}

func TestObjectToEventRequiresIdentity(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "node-disk-node-a-disk1"},
		"spec": map[string]any{
			"diskID": "disk1",
		},
	}}
	if _, ok := objectToEvent(obj, NodeDiskEventModified); ok {
		t.Fatal("objectToEvent() should fail when spec.nodeName is missing")
	}
}

func TestToNodeDiskEventType(t *testing.T) {
	tests := []struct {
		name string
		in   watch.EventType
		want NodeDiskEventType
	}{
		{name: "added", in: watch.Added, want: NodeDiskEventAdded},
		{name: "modified", in: watch.Modified, want: NodeDiskEventModified},
		{name: "deleted", in: watch.Deleted, want: NodeDiskEventDeleted},
		{name: "bookmark fallback", in: watch.Bookmark, want: NodeDiskEventModified},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toNodeDiskEventType(tt.in)
			if got != tt.want {
				t.Fatalf("toNodeDiskEventType()=%q, want %q", got, tt.want)
			}
		})
	}
}
