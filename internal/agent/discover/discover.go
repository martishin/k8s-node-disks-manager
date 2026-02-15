package discover

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	v1alpha1 "github.com/martishin/k8s-node-disks-manager/api/v1alpha1"
	agentmetrics "github.com/martishin/k8s-node-disks-manager/internal/agent/metrics"
	"github.com/martishin/k8s-node-disks-manager/internal/agent/store"
	"github.com/martishin/k8s-node-disks-manager/internal/shared"
)

type Runner struct {
	NodeName       string
	ContainerDir   string
	HostDir        string
	ScanPeriod     time.Duration
	Store          store.Store
	Metrics        *agentmetrics.Collector
	Logger         *slog.Logger
	defaultDesired v1alpha1.NodeDiskDesiredSpec

	seen map[string]struct{}
}

func (r *Runner) Run(ctx context.Context) error {
	if r.ScanPeriod <= 0 {
		r.ScanPeriod = 15 * time.Second
	}
	if r.Logger == nil {
		r.Logger = slog.Default()
	}
	if r.defaultDesired.State == "" {
		r.defaultDesired.State = v1alpha1.DesiredStateAvailable
	}
	if r.seen == nil {
		r.seen = map[string]struct{}{}
	}

	ticker := time.NewTicker(r.ScanPeriod)
	defer ticker.Stop()

	for {
		if err := r.scanOnce(ctx); err != nil {
			r.Logger.Error("scan failed", "error", err)
			if r.Metrics != nil {
				r.Metrics.IncScanError()
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (r *Runner) scanOnce(ctx context.Context) error {
	matches, err := filepath.Glob(filepath.Join(r.ContainerDir, "*.img"))
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	current := map[string]struct{}{}

	for _, filePath := range matches {
		info, err := os.Stat(filePath)
		if err != nil {
			continue
		}
		diskID := strings.TrimSuffix(filepath.Base(filePath), ".img")
		if diskID == "" {
			continue
		}
		name := shared.NodeDiskName(r.NodeName, diskID)
		hostPath := filepath.Join(r.HostDir, filepath.Base(filePath))
		current[name] = struct{}{}

		if err := r.Store.UpsertNodeDisk(ctx, store.NodeDiskUpsert{
			Name:     name,
			NodeName: r.NodeName,
			DiskID:   diskID,
			Path:     hostPath,
			Labels: map[string]string{
				shared.LabelNode: r.NodeName,
				shared.LabelDisk: diskID,
			},
			DefaultTo: r.defaultDesired,
		}); err != nil {
			return err
		}

		phase := v1alpha1.NodePhaseAvailable
		if hasReservedMarker(filePath) {
			phase = v1alpha1.NodePhaseReserved
		}
		capacity := info.Size()
		if err := r.Store.PatchNodeStatus(ctx, name, store.NodeStatusPatch{
			Phase:         phase,
			CapacityBytes: &capacity,
			LastSeenTime:  now,
			Message:       fmt.Sprintf("disk %s discovered", diskID),
		}); err != nil {
			return err
		}
	}

	for previously := range r.seen {
		if _, ok := current[previously]; ok {
			continue
		}
		_ = r.Store.PatchNodeStatus(ctx, previously, store.NodeStatusPatch{
			Phase:        v1alpha1.NodePhaseMissing,
			LastSeenTime: now,
			Message:      "disk file no longer present",
		})
	}

	r.seen = current
	if r.Metrics != nil {
		r.Metrics.SetDisksTotal(len(matches))
		r.Metrics.SetLastScan(now)
	}
	return nil
}

func hasReservedMarker(filePath string) bool {
	_, err := os.Stat(filePath + ".reserved")
	return err == nil
}
