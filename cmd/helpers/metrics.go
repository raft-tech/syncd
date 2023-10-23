/*
 * Copyright (c) 2023. Raft, LLC
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package helpers

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func Health() (*Probes, error) {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	return &Probes{
		Registry: reg,
	}, nil
}

type Probes struct {
	Registry *prometheus.Registry
	ready    bool
}

func (mc *Probes) Ready() {
	mc.ready = true
}

func (mc *Probes) NotReady() {
	mc.ready = false
}

func (mc *Probes) Serve(addr string) *MetricServerContext {
	msc := &MetricServerContext{
		http: http.Server{
			Addr:    addr,
			Handler: mc.Http(),
		},
	}
	var listener net.Listener
	if l, err := net.Listen("tcp", addr); err == nil {
		listener = l
	} else {
		msc.err = err
		return msc
	}
	go func() {
		if e := msc.http.Serve(listener); !errors.Is(e, http.ErrServerClosed) {
			msc.err = e
		}
	}()
	return msc
}

type MetricServerContext struct {
	http http.Server
	err  error
}

func (msc *MetricServerContext) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if e := msc.http.Shutdown(ctx); e != nil {
		_ = msc.http.Close()
	}
}

func (msc *MetricServerContext) Error() error {
	return msc.err
}

func (mc *Probes) Http() *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/healthz", http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.WriteHeader(http.StatusOK)
	}))
	mux.Handle("/healthz/ready", http.HandlerFunc(func(res http.ResponseWriter, _ *http.Request) {
		if mc.ready {
			io.WriteString(res, "READY")
		} else {
			res.WriteHeader(http.StatusServiceUnavailable)
			io.WriteString(res, "NOT READY")
		}
	}))
	mux.Handle("/metrics", promhttp.HandlerFor(mc.Registry, promhttp.HandlerOpts{}))
	return mux
}
