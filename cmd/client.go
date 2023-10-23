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
	"fmt"
	"sync"
	"time"

	"github.com/raft-tech/syncd/cmd/helpers"
	"github.com/raft-tech/syncd/internal/log"
	"github.com/raft-tech/syncd/pkg/client"
	"github.com/raft-tech/syncd/pkg/graph"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func NewPush() *cobra.Command {
	cmd := &cobra.Command{
		RunE:  PushOrPull,
		Short: "Push data to remote peers",
		Use:   "push [FLAGS]",
	}
	cmd.PersistentFlags().Duration("continuous", 0, "--continuous DURATION; push continuously, pausing DURATION between")
	return cmd
}

func NewPull() *cobra.Command {
	cmd := &cobra.Command{
		RunE:  PushOrPull,
		Short: "Pull data from remote peers",
		Use:   "pull [FLAGS]",
	}
	cmd.PersistentFlags().Duration("continuous", 0, "--continuous DURATION; pull continuously, pausing DURATION between")
	return cmd
}

func PushOrPull(cmd *cobra.Command, args []string) error {

	ctx := cmd.Context()

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

	var continuous time.Duration
	if c, e := cmd.Flags().GetDuration("continuous"); c > 0 && e == nil {
		continuous = c
		logger.Info("continuous enabled", zap.Duration("continuous", c))
	} else if e != nil {
		return fail("error setting continuous pause duration", e)
	}

	var clientConfig Client
	if err := config.Sub("client").Unmarshal(&clientConfig); err == nil {
		if err = clientConfig.Validate(); err != nil {
			return fail("invalid client configuration", err)
		}
	} else {
		return fail("error parsing client configuration", err)
	}

	var dopt []grpc.DialOption

	if tcfg, err := helpers.ClientTLSConfig(clientConfig.Auth.TLS); err == nil {
		dopt = append(dopt, grpc.WithTransportCredentials(credentials.NewTLS(tcfg)))
	} else {
		return fail("error creating TLS credentials", err)
	}

	if psk := clientConfig.Auth.PreSharedKey.GetValue(); psk != "" {
		dopt = append(dopt, client.WithPreSharedKey(psk))
	}

	copt := []client.ClientOption{client.WithDialOptions(dopt...)}
	if addr := config.GetString("client.metrics.listen"); addr != "" {
		if continuous == 0 {
			logger.Info("metrics server enabled but --continuous not set")
		}
		var health *helpers.Probes
		if h, err := helpers.Health(); err == nil {
			health = h
		} else {
			return fail("error configuring health probes", err)
		}
		copt = append(copt, client.WithMetrics(health.Registry))
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

	var graphs map[string]graph.Graph
	var closer func(context.Context) error
	logger.Debug("initializing graph")
	if g, c, err := helpers.Graph(ctx, config.Sub("graph")); err == nil {
		graphs = g
		closer = c
		defer func() {
			if e := closer(cmd.Context()); e != nil {
				logger.Error("error closing graph factory", zap.Error(e))
			}
		}()
	} else {
		return fail("error initializing graph", err)
	}
	logger.Info("graph initialized")

	var err error
	wg := sync.WaitGroup{}
	for name, peer := range clientConfig.Peers {
		wg.Add(1)
		go func(ctx context.Context, name string, peer *Peer) {
			defer wg.Done()
			logger := log.FromContext(ctx).With(zap.String("peer", name), zap.String("as", clientConfig.Name))
			ctx = log.NewContext(ctx, logger)
			defer func() {
				if r := recover(); r != nil {
					logger := logger.WithOptions(zap.AddCallerSkip(1))
					if s, ok := r.(string); ok {
						logger.Error("panicked while syncing with peer", zap.String("panic", s))
					} else {
						logger.Error("panicked while syncing with peer")
					}
					err = helpers.NewError("panicked while syncing with "+name, 1)
				}
			}()

			popts := copt
			if peer.Authority != "" {
				popts = make([]client.ClientOption, 0, len(copt)+1)
				popts = append(popts, copt...)
				popts = append(popts, client.WithDialOptions(grpc.WithAuthority(peer.Authority)))
			}

			syncd, e := client.New(ctx, peer.Address, popts...)
			if e != nil {
				logger.Error("error initializing client", zap.Error(e))
				err = e
				return
			}
			if e = syncd.Connect(ctx); e == nil {
				defer func() {
					if e = syncd.Close(); e != nil {
						logger.Error("error closing peer connection", zap.Error(err))
						err = e
					}
				}()
			} else {
				logger.Error("error connecting to peer", zap.Error(e))
				err = e
				return
			}

			switch cmd.CalledAs() {
			case "push":
				req := pushRequest{
					client: syncd,
					as:     clientConfig.Name,
					models: peer.PushModels,
					graphs: graphs,
				}
				if continuous > 0 {
					for done := false; !done; {
						_ = doPush(ctx, req)
						wait := time.NewTimer(continuous)
						select {
						case <-wait.C:
						case <-ctx.Done():
							done = true
							if !wait.Stop() {
								<-wait.C
							}
						}
					}
				} else if e = doPush(ctx, req); e != nil {
					err = e
				}
			case "pull":
				req := pullRequest{
					client: syncd,
					as:     clientConfig.Name,
					models: peer.PullModels,
					graphs: graphs,
				}
				if continuous > 0 {
					for done := false; !done; {
						_ = doPull(ctx, req)
						wait := time.NewTimer(continuous)
						select {
						case <-wait.C:
						case <-ctx.Done():
							done = true
							if !wait.Stop() {
								<-wait.C
							}
						}
					}
				} else if e = doPull(ctx, req); e != nil {
					err = e
				}
			default:
				panic("unrecognized action " + cmd.CalledAs())
			}
		}(ctx, name, peer)
	}

	wg.Wait()
	return err
}

type pushRequest struct {
	client client.Client
	as     string
	models map[string]*PushModel
	graphs map[string]graph.Graph
}

func doPush(ctx context.Context, req pushRequest) (err error) {

	logger := log.FromContext(ctx)
	logger.Info("starting sync", zap.String("via", "push"))

	// Watch for a canceled context
	var done bool
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		<-ctx.Done()
		done = true
	}()

	for name, model := range req.models {
		if done {
			logger.Info("push aborted due to canceled context")
			break
		}
		if g, ok := req.graphs[name]; ok {
			logger := logger.With(zap.String("model", name))
			if e := req.client.Push(ctx, name, g.Source(name, model.Filters...), req.as); e != nil {
				logger.Error("error pulling from peer", zap.Error(e))
				err = e
			}
		} else {
			logger.Error("undefined push model", zap.String("model", name))
			err = helpers.NewError("undefined model used in peer push configuration", 2)
		}
	}
	if !done {
		logger.Info("sync completed", zap.String("via", "push"), zap.Bool("withErrors", err != nil))
	}
	return
}

type pullRequest struct {
	client client.Client
	as     string
	models []string
	graphs map[string]graph.Graph
}

func doPull(ctx context.Context, req pullRequest) (err error) {

	logger := log.FromContext(ctx)
	logger.Info("starting sync", zap.String("via", "pull"))

	// Watch for a canceled context
	var done bool
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		<-ctx.Done()
		done = true
	}()

	// Pull all models
	for _, model := range req.models {
		if done {
			logger.Info("pull aborted due to canceled context")
			break
		}
		if g, ok := req.graphs[model]; ok {
			if e := req.client.Pull(ctx, model, g.Destination(), req.as); e != nil {
				logger.Error("error pulling from peer", zap.Error(e))
				err = e
			}
		} else {
			logger.Error("unknown pull model", zap.String("model", model))
			err = helpers.NewError("undefined model used in peer pull configuration", 2)
		}
	}
	if !done {
		logger.Info("completed sync", zap.String("via", "pull"), zap.Bool("withErrors", err != nil))
	}

	return
}

type Client struct {
	Name  string
	Peers map[string]*Peer
	Auth  struct {
		PreSharedKey helpers.StringValue
		TLS          helpers.TLS
	}
}

func (c *Client) Validate() error {
	// TODO validate Client
	return nil
}

type Peer struct {
	Address    string
	Authority  string
	PullModels []string              `mapstructure:"pull"`
	PushModels map[string]*PushModel `mapstructure:"push"`
}

type PushModel struct {
	Filters []graph.Filter
}
