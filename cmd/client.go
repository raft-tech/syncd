package cmd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
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
	if c, err := cmd.Flags().GetDuration("continuous"); c > 0 && err == nil {
		continuous = c
		logger.Info("continuous enabled", zap.Duration("continuous", c))
		if addr := config.GetString("client.metrics.listen"); addr != "" {
			var health *helpers.Probes
			if health, err = helpers.Health(); err != nil {
				return fail("error configuring metrics server", err)
			}
			wg := sync.WaitGroup{}
			wg.Add(1)
			go func() {
				if listener, e := net.Listen("tcp", addr); err == nil {
					wg.Done()
					logger.Debug("started metrics server", zap.String("addr", addr))
					_ = http.Serve(listener, health.Http())
				} else {
					err = e
					wg.Done()
				}
			}()
			wg.Wait()
			if err != nil {
				return fail("error starting metrics server", err)
			}
		}
	}

	var clientConfig Client
	if err := config.Unmarshal(&clientConfig); err == nil {
		if err = clientConfig.Validate(); err != nil {
			return fail("invalid client configuration", err)
		}
	} else {
		return fail("error parsing client configuration", err)
	}

	copt := []client.ClientOption{client.WithDialOptions(clientConfig.DialOptions()...)}
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
	if g, c, err := helpers.Graph(cmd.Context(), config.Sub("graph")); err == nil {
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

	errs := make(chan error)
	defer close(errs)
	wg := sync.WaitGroup{}
	for name, peer := range clientConfig.Peers {
		wg.Add(1)
		go func(ctx context.Context, name string, peer *Peer, out chan<- error) {

			logger := log.FromContext(ctx).With(zap.String("peer", name), zap.String("as", clientConfig.Name))
			ctx = log.NewContext(ctx, logger)
			defer func() {
				if r := recover(); r != nil {
					logger.Error("recovered from panic")
					out <- helpers.NewError("panicked while syncing with "+name, 1)
				}
				wg.Done()
			}()

			syncd, err := client.New(ctx, peer.Address, copt...)
			if err != nil {
				logger.Error("error initializing client", zap.Error(err))
				return
			}
			if err = syncd.Connect(ctx); err == nil {
				defer func() {
					if err = syncd.Close(); err != nil {
						logger.Error("error closing peer connection", zap.Error(err))
					}
				}()
			} else {
				logger.Error("error connecting to peer", zap.Error(err))
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
					wait := time.NewTimer(continuous)
					for done := false; !done; {
						_ = doPush(ctx, req)
						select {
						case <-wait.C:
						case <-ctx.Done():
							done = true
							wait.Stop()
							<-wait.C
						}
					}
				} else if err := doPush(ctx, req); err != nil {
					out <- err
				}
			case "pull":
				req := pullRequest{
					client: syncd,
					as:     clientConfig.Name,
					models: nil,
					graphs: nil,
				}
				if continuous > 0 {
					for done := false; !done; {
						_ = doPull(ctx, req)
						wait := time.NewTimer(continuous)
						select {
						case <-wait.C:
						case <-ctx.Done():
							done = true
							wait.Stop()
							<-wait.C
						}
					}
				} else if err := doPull(ctx, req); err != nil {
					out <- err
				}
			default:
				panic("unrecognized action " + cmd.CalledAs())
			}
		}(cmd.Context(), name, peer, errs)
	}
	var err error
	for e := range errs {
		err = e
	}
	wg.Wait()
	return err
}

type pushRequest struct {
	client client.Client
	as     string
	models map[string][]graph.Filter
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

	for model, filters := range req.models {
		if done {
			logger.Info("push aborted due to canceled context")
			break
		}
		if g, ok := req.graphs[model]; ok {
			logger := logger.With(zap.String("model", model))
			if e := req.client.Push(ctx, model, g.Source(model, filters...), req.as); e != nil {
				logger.Error("error pulling from peer", zap.Error(e))
				err = e
			}
		} else {
			logger.Error("undefined push model", zap.String("model", model))
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
	Name         string
	Peers        map[string]*Peer
	PreSharedKey struct {
		FromEnv string
		Value   string
	}
	TLS helpers.TLS
}

func (c *Client) Validate() error {
	// TODO validate Client
	return nil
}

func (c *Client) DialOptions() (opt []grpc.DialOption) {
	psk := c.PreSharedKey.Value
	if e := c.PreSharedKey.FromEnv; psk == "" && e != "" {
		psk = os.Getenv(e)
	}
	if psk != "" {
		opt = append(opt, client.WithPreSharedKey(psk))
	}
	return
}

type Peer struct {
	Address    string
	Authority  string
	PullModels []string
	PushModels map[string][]graph.Filter
}
