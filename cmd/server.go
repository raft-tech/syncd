package cmd

import (
	"context"
	"crypto/x509"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/raft-tech/syncd/cmd/helpers"
	"github.com/raft-tech/syncd/internal/api"
	"github.com/raft-tech/syncd/internal/log"
	"github.com/raft-tech/syncd/pkg/server"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func NewServer() *cobra.Command {
	return &cobra.Command{
		RunE:  Serve,
		Short: "Serve data to remote peers",
		Use:   "serve",
	}
}

func Serve(cmd *cobra.Command, args []string) error {

	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	var config *viper.Viper
	if c, err := helpers.Config(cmd); err == nil {
		config = c
	} else {
		fmt.Printf("error parsing config file: %v\n", err)
		return err
	}

	var logger *zap.Logger
	if l, err := helpers.Logger(cmd.OutOrStdout(), config.Sub("logging")); err == nil {
		logger = l
		ctx = log.NewContext(ctx, l)
	} else {
		fmt.Printf("error initializing logger: %v\n", err)
		return err
	}

	fail := func(msg string, err error) error {
		logger.Error(msg, zap.Error(err))
		return err
	}

	var sopts []grpc.ServerOption
	if config.IsSet("server.tls.crt") {
		if cfg, e := helpers.ServerTLSConfig(config.Sub("server.tls")); e == nil {
			sopts = append(sopts, grpc.Creds(credentials.NewTLS(cfg)))
			logger := logger
			if len(cfg.Certificates) > 0 {
				if crt, err := x509.ParseCertificate(cfg.Certificates[0].Certificate[0]); err == nil {
					logger = logger.With(zap.String("subject", crt.Subject.String()))
				}
			}
			logger.Debug("TLS enabled", zap.String("TLSClientAuth", cfg.ClientAuth.String()))
		} else {
			return fail("error configuring tls", e)
		}
	}

	if ggraph, closer, e := helpers.Graph(ctx, config.Sub("graph")); e == nil {
		defer func() {
			// ctx will most likely be canceled, so use the original cmd.Context()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			ctx = log.NewContext(ctx, logger)
			if e := closer(ctx); e != nil {
				logger.Error("error closing graphs", zap.Error(e))
			}
		}()
		for k, v := range ggraph {
			server.RegisterGraph(k, v)
		}
	} else {
		return fail("error initializing graph", e)
	}

	uics := []grpc.UnaryServerInterceptor{
		func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
			ctx = log.NewContext(ctx, logger)
			return handler(ctx, req)
		},
	}
	sics := []grpc.StreamServerInterceptor{
		func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			ctx := log.NewContext(ss.Context(), logger)
			return handler(srv, server.ServerStreamWithContext(ss, ctx))
		},
	}
	if config.IsSet("server.auth.presharedkey") {
		psk := helpers.StringValue{}
		if err := config.UnmarshalKey("server.auth.presharedkey", &psk); err == nil {
			if k := psk.GetValue(); k != "" {
				logger.Debug("presharedkey authentication enabled")
				auth := server.PreSharedKey(psk.GetValue())
				uics = append(uics, auth.UnaryInterceptor)
				sics = append(sics, auth.StreamInterceptor)
			}
		}
	}
	sopts = append(sopts, grpc.ChainUnaryInterceptor(uics...), grpc.ChainStreamInterceptor(sics...))

	srv := grpc.NewServer(sopts...)
	api.RegisterSyncServer(srv, server.New(server.DefaultOptions))

	var listener net.Listener
	if l, err := net.Listen("tcp", config.GetString("server.listen")); err == nil {
		listener = l
	} else {
		return fail("error starting TCP listener", err)
	}

	srvErr := make(chan error)
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func(out chan<- error) {
		defer wg.Done()
		defer close(out)
		logger.Info("starting server", zap.String("address", listener.Addr().String()))
		if e := srv.Serve(listener); e != nil {
			out <- e
		}
	}(srvErr)

	select {
	case err := <-srvErr:
		return fail("server error", err)
	case <-ctx.Done():
		logger.Info("shutting down")
		cancel()
		srv.GracefulStop()
	}
	wg.Wait()
	logger.Info("shutdown complete")
	return nil
}
