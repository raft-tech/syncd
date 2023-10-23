/*
 *     Copyright (c) 2023. Raft LLC
 *
 *     This program is free software: you can redistribute it and/or modify
 *     it under the terms of the GNU General Public License as published by
 *     the Free Software Foundation, either version 3 of the License, or
 *     (at your option) any later version.
 *
 *     This program is distributed in the hope that it will be useful,
 *     but WITHOUT ANY WARRANTY; without even the implied warranty of
 *     MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 *     GNU General Public License for more details.
 *
 *     You should have received a copy of the GNU General Public License
 *     along with this program.  If not, see <https://www.gnu.org/licenses/>.
 *
 */

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

type Collector interface {
	WithLabels(labels prometheus.Labels) RequestMetricsCollector
}

func ForClient(reg prometheus.Registerer) Collector {
	if reg == nil {
		return nop{}
	}
	c := new(collector)
	c.register(reg, "client")
	return c
}

type ServerCollector interface {
	Collector
	InvalidModel()
	InvalidPeerModel(peer string)
}

func ForServer(reg prometheus.Registerer) ServerCollector {
	if reg == nil {
		return nop{}
	}
	c := new(collector)
	c.register(reg, "server")
	c.unknownModels = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "syncd",
		Subsystem: "server",
		Name:      "unknown_models",
	})
	c.invalidPeerModels = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "syncd",
		Subsystem: "server",
		Name:      "invalid_peer_models",
	}, []string{"peer"})
	return c
}

type collector struct {
	unknownModels       prometheus.Counter
	invalidPeerModels   *prometheus.CounterVec
	errors              *prometheus.CounterVec
	checks              *prometheus.CounterVec
	recordsPushed       *prometheus.CounterVec
	recordsPulled       *prometheus.CounterVec
	recordsAcknowledged *prometheus.CounterVec
}

func (col *collector) register(reg prometheus.Registerer, subsystem string) {
	labels := []string{"peer", "model"}
	col.errors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "syncd",
		Subsystem: subsystem,
		Name:      "errors",
	}, labels)
	col.checks = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "syncd",
		Subsystem: subsystem,
		Name:      "checks",
	}, labels)
	col.recordsPulled = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "syncd",
		Subsystem: subsystem,
		Name:      "records_pulled",
	}, labels)
	col.recordsPushed = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "syncd",
		Subsystem: subsystem,
		Name:      "records_pushed",
	}, labels)
	col.recordsAcknowledged = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "syncd",
		Subsystem: subsystem,
		Name:      "records_acknowledged",
	}, labels)
}

func (col *collector) InvalidModel() {
	col.unknownModels.Inc()
}

func (col *collector) InvalidPeerModel(peer string) {
	col.invalidPeerModels.With(prometheus.Labels{"peer": peer}).Inc()
}

func (col *collector) WithLabels(labels prometheus.Labels) RequestMetricsCollector {
	metrics := new(requestMetrics)
	metrics.errors = col.errors.With(labels)
	metrics.checked = col.checks.With(labels)
	metrics.recordsPushed = col.recordsPushed.With(labels)
	metrics.recordsPulled = col.recordsPulled.With(labels)
	metrics.recordsAcknowledged = col.recordsAcknowledged.With(labels)
	return metrics
}

type RequestMetricsCollector interface {
	Erred()
	Checked()
	Pushed()
	Pulled()
	Acknowledged()
}

type requestMetrics struct {
	errors              prometheus.Counter
	checked             prometheus.Counter
	recordsPushed       prometheus.Counter
	recordsPulled       prometheus.Counter
	recordsAcknowledged prometheus.Counter
}

func (r *requestMetrics) Erred() {
	r.errors.Inc()
}

func (r *requestMetrics) Checked() {
	r.checked.Inc()
}

func (r *requestMetrics) Pushed() {
	r.recordsPushed.Inc()
}

func (r *requestMetrics) Pulled() {
	r.recordsPulled.Inc()
}

func (r *requestMetrics) Acknowledged() {
	r.recordsAcknowledged.Inc()
}

type nop struct{}

func (n nop) WithLabels(labels prometheus.Labels) RequestMetricsCollector {
	return n
}

func (n nop) InvalidModel() {}

func (n nop) InvalidPeerModel(peer string) {}

func (n nop) Erred() {}

func (n nop) Checked() {}

func (n nop) Pushed() {}

func (n nop) Pulled() {}

func (n nop) Acknowledged() {}
