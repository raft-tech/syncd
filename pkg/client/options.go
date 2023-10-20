package client

import (
	"context"
	"crypto/tls"

	"github.com/raft-tech/syncd/pkg/server"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type ClientOption func(c *client) error

func WithTLS(config *tls.Config) ClientOption {
	cfg := config
	return func(c *client) error {
		c.dialOpts = append(c.dialOpts, grpc.WithTransportCredentials(credentials.NewTLS(cfg)))
		return nil
	}
}

func WithDialOptions(opt ...grpc.DialOption) ClientOption {
	return func(c *client) error {
		c.dialOpts = append(c.dialOpts, opt...)
		return nil
	}
}

func WithPreSharedKey(key string) grpc.DialOption {
	if key == "" {
		panic("key must not be empty")
	}
	return grpc.WithPerRPCCredentials(preSharedKey(key))
}

type preSharedKey string

func (psk preSharedKey) GetRequestMetadata(_ context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{server.PSK_METADATA_KEY: string(psk)}, nil
}

func (_ preSharedKey) RequireTransportSecurity() bool {
	return true
}
