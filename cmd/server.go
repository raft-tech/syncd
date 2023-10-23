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
		defer func() { _ = l.Sync() }()
	} else {
		fmt.Printf("error initializing logger: %v\n", err)
		return err
	}

	fail := func(msg string, err error) error {
		logger.Error(msg, zap.Error(err))
		return err
	}

	var health *helpers.Probes
	if h, err := helpers.Health(); err == nil {
		health = h
	} else {
		return fail("error initializing health probes", err)
	}

	// Start the metrics server if configured
	if addr := config.GetString("server.metrics.listen"); addr != "" {
		logger.Debug("starting metric server", zap.String("addr", addr))
		msc := health.Serve(addr)
		if err := msc.Error(); err == nil {
			logger.Debug("metric server started", zap.String("addr", addr))
			defer func() {
				logger.Debug("stopping metric server")
				msc.Stop()
				if e := msc.Error(); e == nil {
					logger.Debug("metric server stopped")
				} else {
					logger.Error("metric server err")
				}
			}()
		} else {
			return fail("error starting metrics server", err)
		}
	}

	var grpcOpts []grpc.ServerOption
	if config.IsSet("server.tls.crt") {
		if cfg, e := helpers.ServerTLSConfig(config.Sub("server.tls")); e == nil {
			grpcOpts = append(grpcOpts, grpc.Creds(credentials.NewTLS(cfg)))
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

	var models helpers.ModelMap
	if m, err := helpers.Models(config.Sub("server.models")); err == nil {
		models = m
	} else {
		return fail("error parsing server models", err)
	}

	if ggraph, closer, e := helpers.Graph(ctx, config.Sub("graph")); e == nil {
		defer func() {
			// ctx will most likely be canceled, so use background context
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			ctx = log.NewContext(ctx, logger)
			logger.Debug("closing graphs")
			if e := closer(ctx); e == nil {
				logger.Debug("graphs closed")
			} else {
				logger.Error("error closing graphs", zap.Error(e))
			}
		}()
		for k, v := range ggraph {
			if m, ok := models[k]; ok {
				server.RegisterGraph(k, v)
				server.RegisterFilters(k, m.Filters...)
			}
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
	grpcOpts = append(grpcOpts, grpc.ChainUnaryInterceptor(uics...), grpc.ChainStreamInterceptor(sics...))

	srv := grpc.NewServer(grpcOpts...)
	api.RegisterSyncServer(srv, server.New(server.Options{Metrics: health.Registry}))

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
		logger.Info("server stopped")
	}(srvErr)
	logger.Debug("ready")
	health.Ready()

	select {
	case err := <-srvErr:
		wg.Wait()
		return fail("server error", err)
	case <-ctx.Done():
		health.NotReady()
		logger.Info("shutting down server")
		cancel()
		srv.GracefulStop()
	}
	wg.Wait()
	return nil
}
