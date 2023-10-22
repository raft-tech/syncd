package api

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc/metadata"
)

func GetMetadataFromContext(ctx context.Context) (md Metadata, ok bool) {
	var rmd metadata.MD
	if rmd, ok = metadata.FromIncomingContext(ctx); !ok {
		return
	}
	if p := rmd.Get("peer"); len(p) == 1 {
		md.Peer = p[0]
	} else {
		ok = false
		return
	}
	if m := rmd.Get("model"); len(m) == 1 {
		md.Model = m[0]
	} else {
		ok = false
		return
	}
	return
}

func MustGetMetadataFromContext(ctx context.Context) Metadata {
	md, ok := GetMetadataFromContext(ctx)
	if !ok {
		panic("invalid request metadata")
	}
	return md
}

type Metadata struct {
	Peer   string
	Model  string
	labels prometheus.Labels
}

func (md *Metadata) MetricLabels() prometheus.Labels {
	if md.labels == nil {
		md.labels = map[string]string{
			"peer":  md.Peer,
			"model": md.Model,
		}
	}
	return md.labels
}
