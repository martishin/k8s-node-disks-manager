package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/martishin/k8s-node-disks-manager/internal/agent/discover"
	agentmetrics "github.com/martishin/k8s-node-disks-manager/internal/agent/metrics"
	agentreconcile "github.com/martishin/k8s-node-disks-manager/internal/agent/reconcile"
	"github.com/martishin/k8s-node-disks-manager/internal/agent/store"
	"github.com/martishin/k8s-node-disks-manager/internal/shared"
	"golang.org/x/sync/errgroup"
)

func main() {
	var scanPeriod time.Duration
	var diskDir string
	var hostRoot string
	var mountRoot string
	var metricsAddr string

	flag.DurationVar(&scanPeriod, "scan-period", 15*time.Second, "scan period for discovering disk files")
	flag.StringVar(&diskDir, "disk-dir", shared.DefaultDiskDirContainer, "container path to disk files")
	flag.StringVar(&hostRoot, "host-root-path", shared.DefaultHostRootPath, "host root path mounted into the agent")
	flag.StringVar(&mountRoot, "mount-root-path", shared.DefaultContainerRoot, "container mount root path")
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":2112", "metrics bind address")
	flag.Parse()

	nodeName := os.Getenv("NODE_NAME")
	if nodeName == "" {
		slog.Error("NODE_NAME environment variable is required")
		os.Exit(1)
	}

	kubeStore, err := store.NewKubeStore()
	if err != nil {
		slog.Error("failed to initialize kube store", "error", err)
		os.Exit(1)
	}

	metrics := agentmetrics.New(nodeName)
	server := &http.Server{Addr: metricsAddr, Handler: metrics.Handler()}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	events, err := kubeStore.WatchNodeDisks(ctx, nodeName)
	if err != nil {
		slog.Error("failed to start NodeDisk watch", "error", err)
		os.Exit(1)
	}

	discoverRunner := &discover.Runner{
		NodeName:     nodeName,
		ContainerDir: diskDir,
		HostDir:      hostRoot + "/disks",
		ScanPeriod:   scanPeriod,
		Store:        kubeStore,
		Metrics:      metrics,
		Logger:       slog.Default(),
	}
	applyRunner := &agentreconcile.Runner{
		NodeName:      nodeName,
		HostRootPath:  hostRoot,
		MountRootPath: mountRoot,
		Store:         kubeStore,
		Metrics:       metrics,
		Logger:        slog.Default(),
	}

	grp, grpCtx := errgroup.WithContext(ctx)
	grp.Go(func() error {
		slog.Info("agent metrics server", "addr", metricsAddr)
		err := server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})
	grp.Go(func() error {
		return discoverRunner.Run(grpCtx)
	})
	grp.Go(func() error {
		return applyRunner.Run(grpCtx, events)
	})
	grp.Go(func() error {
		<-grpCtx.Done()
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelShutdown()
		return server.Shutdown(shutdownCtx)
	})

	if err := grp.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("agent stopped with error", "error", err)
		os.Exit(1)
	}
}
