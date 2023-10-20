package server

import (
	"context"
	"sync"

	"github.com/raft-tech/syncd/pkg/graph"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type GraphResolver func(string) (graph.Graph, bool)

var (
	graphs                             = make(map[string]graph.Graph)
	graphsLock                         = sync.Mutex{}
	defaultGraphResolver GraphResolver = func(k string) (graph.Graph, bool) {
		g, ok := graphs[k]
		return g, ok
	}
	DefaultOptions = Options{}
)

func RegisterGraph(key string, graph graph.Graph) bool {
	graphsLock.Lock()
	defer graphsLock.Unlock()
	if _, ok := graphs[key]; !ok {
		graphs[key] = graph
		return true
	} else {
		return false
	}
}

func DeregisterGraph(key string) bool {
	graphsLock.Lock()
	defer graphsLock.Unlock()
	if _, ok := graphs[key]; ok {
		delete(graphs, key)
		return true
	} else {
		return false
	}
}

type PeerValidator func(string, string) bool

var defaultPeerValidator = func(string, string) bool {
	return true
}

type Options struct {
	GraphResolver GraphResolver
	PeerValidator PeerValidator
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
}

const (
	PSK_METADATA_KEY = "x-syncd-presharedkey"
)

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
