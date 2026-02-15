package reconcile

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
)

type Runner struct {
	NodeName      string
	HostRootPath  string
	MountRootPath string
	Store         store.Store
	Metrics       *agentmetrics.Collector
	Logger        *slog.Logger
}

func (r *Runner) Run(ctx context.Context, events <-chan store.NodeDiskEvent) error {
	if r.Logger == nil {
		r.Logger = slog.Default()
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case evt, ok := <-events:
			if !ok {
				return nil
			}
			if evt.NodeName != r.NodeName {
				continue
			}
			if evt.Type == store.NodeDiskEventDeleted {
				continue
			}
			if err := r.applyDesired(ctx, evt); err != nil {
				r.Logger.Error("apply desired failed", "name", evt.Name, "error", err)
			}
		}
	}
}

func (r *Runner) applyDesired(ctx context.Context, evt store.NodeDiskEvent) error {
	path := toContainerPath(evt.Path, r.HostRootPath, r.MountRootPath)
	if path == "" {
		return fmt.Errorf("empty path for %s", evt.Name)
	}
	marker := path + ".reserved"
	now := time.Now().UTC()

	switch evt.DesiredState {
	case v1alpha1.DesiredStateReserved:
		err := os.WriteFile(marker, []byte("reserved\n"), 0o644)
		if err != nil {
			r.markAction("reserve", false)
			_ = r.Store.PatchNodeStatus(ctx, evt.Name, store.NodeStatusPatch{
				Phase:        v1alpha1.NodePhaseError,
				LastSeenTime: now,
				Message:      err.Error(),
			})
			return err
		}
		r.markAction("reserve", true)
		return r.Store.PatchNodeStatus(ctx, evt.Name, store.NodeStatusPatch{
			Phase:        v1alpha1.NodePhaseReserved,
			LastSeenTime: now,
			Message:      "disk reserved",
		})
	default:
		err := os.Remove(marker)
		if err != nil && !os.IsNotExist(err) {
			r.markAction("release", false)
			_ = r.Store.PatchNodeStatus(ctx, evt.Name, store.NodeStatusPatch{
				Phase:        v1alpha1.NodePhaseError,
				LastSeenTime: now,
				Message:      err.Error(),
			})
			return err
		}
		r.markAction("release", true)
		return r.Store.PatchNodeStatus(ctx, evt.Name, store.NodeStatusPatch{
			Phase:        v1alpha1.NodePhaseAvailable,
			LastSeenTime: now,
			Message:      "disk available",
		})
	}
}

func (r *Runner) markAction(action string, success bool) {
	if r.Metrics != nil {
		r.Metrics.IncAction(action, success)
	}
}

func toContainerPath(path, hostRoot, mountRoot string) string {
	if path == "" {
		return ""
	}
	if hostRoot != "" && mountRoot != "" && strings.HasPrefix(path, hostRoot) {
		suffix := strings.TrimPrefix(path, hostRoot)
		suffix = strings.TrimPrefix(suffix, "/")
		return filepath.Join(mountRoot, suffix)
	}
	return path
}
