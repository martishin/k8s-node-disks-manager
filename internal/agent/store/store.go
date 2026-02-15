package store

import (
	"context"
	"time"

	v1alpha1 "github.com/martishin/k8s-node-disks-manager/api/v1alpha1"
)

type NodeDiskUpsert struct {
	Name      string
	NodeName  string
	DiskID    string
	Path      string
	Labels    map[string]string
	DefaultTo v1alpha1.NodeDiskDesiredSpec
}

type NodeStatusPatch struct {
	Phase         v1alpha1.NodePhase
	CapacityBytes *int64
	LastSeenTime  time.Time
	Message       string
}

type NodeDiskEventType string

const (
	NodeDiskEventSync     NodeDiskEventType = "Sync"
	NodeDiskEventAdded    NodeDiskEventType = "Added"
	NodeDiskEventModified NodeDiskEventType = "Modified"
	NodeDiskEventDeleted  NodeDiskEventType = "Deleted"
)

type NodeDiskEvent struct {
	Type         NodeDiskEventType
	Name         string
	NodeName     string
	DiskID       string
	Path         string
	DesiredState v1alpha1.DesiredState
	DesiredOwner string
}

type Store interface {
	UpsertNodeDisk(ctx context.Context, nd NodeDiskUpsert) error
	PatchNodeStatus(ctx context.Context, name string, st NodeStatusPatch) error
	WatchNodeDisks(ctx context.Context, nodeName string) (<-chan NodeDiskEvent, error)
}
