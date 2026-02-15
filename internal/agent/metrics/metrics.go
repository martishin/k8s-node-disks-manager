package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Collector struct {
	node string

	disksTotal   *prometheus.GaugeVec
	scanErrors   *prometheus.CounterVec
	lastScanTS   *prometheus.GaugeVec
	actionsTotal *prometheus.CounterVec
	promRegistry *prometheus.Registry
}

func New(node string) *Collector {
	registry := prometheus.NewRegistry()
	factory := promauto.With(registry)

	c := &Collector{
		node: node,
		disksTotal: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "node_disks_agent_disks_total",
			Help: "Number of discovered disks on the node",
		}, []string{"node"}),
		scanErrors: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "node_disks_agent_scan_errors_total",
			Help: "Number of discovery scan errors",
		}, []string{"node"}),
		lastScanTS: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "node_disks_agent_last_scan_timestamp",
			Help: "Unix timestamp of the last discovery scan",
		}, []string{"node"}),
		actionsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "node_disks_agent_actions_total",
			Help: "Node-local desired state actions",
		}, []string{"node", "action", "success"}),
		promRegistry: registry,
	}

	return c
}

func (c *Collector) SetDisksTotal(v int) {
	c.disksTotal.WithLabelValues(c.node).Set(float64(v))
}

func (c *Collector) IncScanError() {
	c.scanErrors.WithLabelValues(c.node).Inc()
}

func (c *Collector) SetLastScan(t time.Time) {
	c.lastScanTS.WithLabelValues(c.node).Set(float64(t.Unix()))
}

func (c *Collector) IncAction(action string, success bool) {
	status := "false"
	if success {
		status = "true"
	}
	c.actionsTotal.WithLabelValues(c.node, action, status).Inc()
}

func (c *Collector) Handler() http.Handler {
	return promhttp.HandlerFor(c.promRegistry, promhttp.HandlerOpts{})
}
