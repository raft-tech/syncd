package client

import (
	"crypto/tls"

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
