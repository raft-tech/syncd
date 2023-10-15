package server

import (
	"sync"

	"github.com/raft-tech/syncd/pkg/graph"
)

type GraphResolver func(string) (graph.Graph, bool)

var (
	graphs                             = make(map[string]graph.Graph)
	graphsLock                         = sync.Mutex{}
	defaultGraphResolver GraphResolver = func(k string) (graph.Graph, bool) {
		g, ok := graphs[k]
		return g, ok
	}
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
