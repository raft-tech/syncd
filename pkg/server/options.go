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

package server

import (
	"context"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/raft-tech/syncd/pkg/graph"
	"github.com/raft-tech/syncd/pkg/metrics"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	PSK_METADATA_KEY = "x-syncd-presharedkey"
)

var (
	graphs                             = make(map[string]graph.Graph)
	graphsLock                         = sync.Mutex{}
	defaultGraphResolver GraphResolver = func(k string) (graph.Graph, bool) {
		g, ok := graphs[k]
		return g, ok
	}
	defaultFilters       Filters       = map[string][]graph.Filter{}
	defaultPeerValidator PeerValidator = func(string, string) bool {
		return true
	}
	DefaultOptions = Options{}
)

type GraphResolver func(string) (graph.Graph, bool)

type PeerValidator func(string, string) bool

type Filters map[string][]graph.Filter

type Options struct {
	GraphResolver GraphResolver
	Filters       Filters
	PeerValidator PeerValidator
	Metrics       prometheus.Registerer
}

func (opt *Options) apply(srv *server) {

	srv.lookup = defaultGraphResolver
	if gr := opt.GraphResolver; gr != nil {
		srv.lookup = gr
	}

	srv.peerValidator = defaultPeerValidator
	if pv := opt.PeerValidator; pv != nil {
		srv.peerValidator = pv
	}

	srv.filters = defaultFilters
	if f := opt.Filters; f != nil {
		srv.filters = f
	}

	srv.metrics = metrics.ForServer(opt.Metrics)
}

func RegisterGraph(model string, graph graph.Graph) bool {
	graphsLock.Lock()
	defer graphsLock.Unlock()
	if _, ok := graphs[model]; !ok {
		graphs[model] = graph
		return true
	} else {
		return false
	}
}

func DeregisterGraph(model string) bool {
	graphsLock.Lock()
	defer graphsLock.Unlock()
	if _, ok := graphs[model]; ok {
		delete(graphs, model)
		return true
	} else {
		return false
	}
}

func RegisterFilters(model string, filters ...graph.Filter) {
	if len(filters) > 0 {
		defaultFilters[model] = append(defaultFilters[model], filters...)
	}
}

func DeregisterAllFilters(model string) (ok bool) {
	if _, ok = defaultFilters[model]; ok {
		delete(defaultFilters, model)
	}
	return
}

type PreSharedKey string

func (psk PreSharedKey) Authn(ctx context.Context) (authz bool) {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if k := md.Get(PSK_METADATA_KEY); len(k) > 0 {
			authz = k[0] == string(psk)
		}
	}
	return authz
}

func (psk PreSharedKey) UnaryInterceptor(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
	if psk.Authn(ctx) {
		return handler(ctx, req)
	} else {
		return nil, status.Error(codes.Unauthenticated, codes.Unauthenticated.String())
	}
}

func (psk PreSharedKey) StreamInterceptor(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	if psk.Authn(ss.Context()) {
		return handler(srv, ss)
	} else {
		return status.Error(codes.Unauthenticated, codes.Unauthenticated.String())
	}
}
