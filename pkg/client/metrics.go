package client

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/raft-tech/syncd/internal/api"
)

type MetricsCollector struct {
	errors              *prometheus.CounterVec
	recordsPushed       *prometheus.CounterVec
	recordsPulled       *prometheus.CounterVec
	recordsAcknowledged *prometheus.CounterVec
}

func (mc *MetricsCollector) For(md api.Metadata) RequestMetricsCollector {
	labels := md.MetricLabels()
	metrics := new(requestMetrics)
	if c := mc.errors; c != nil {
		metrics.errors = c.With(labels)
	}
	if c := mc.recordsPulled; c != nil {
		metrics.recordsPushed = c.With(labels)
	}
	if c := mc.recordsPulled; c != nil {
		metrics.recordsPulled = c.With(labels)
	}
	if c := mc.recordsAcknowledged; c != nil {
		metrics.recordsAcknowledged = c.With(labels)
	}
	return metrics
}

type RequestMetricsCollector interface {
	Erred()
	Pushed()
	Pulled()
	Acknowledged()
}

type requestMetrics struct {
	errors              prometheus.Counter
	recordsPushed       prometheus.Counter
	recordsPulled       prometheus.Counter
	recordsAcknowledged prometheus.Counter
}

func (r *requestMetrics) Erred() {
	if c := r.errors; c != nil {
		c.Inc()
	}
}

func (r *requestMetrics) Pushed() {
	if c := r.recordsPushed; c != nil {
		c.Inc()
	}
}

func (r *requestMetrics) Pulled() {
	if c := r.recordsPulled; c != nil {
		c.Inc()
	}
}

func (r *requestMetrics) Acknowledged() {
	if c := r.recordsAcknowledged; c != nil {
		c.Inc()
	}
}
