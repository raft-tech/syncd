package cmd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"

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

	//var continuous *time.Duration
	if c, err := cmd.Flags().GetDuration("continuous"); c > 0 && err == nil {
		//continuous = &c
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

	var auth ClientAuth
	if err := config.Unmarshal(&auth); err == nil {
		if err = auth.Validate(); err != nil {
			return fail("invalid client auth configuration", err)
		}
	} else {
		return fail("error parsing client auth configuration", err)
	}
	dopt := auth.DialOptions()

	var peers map[string]*Peer
	if err := config.Unmarshal(peers); err == nil {
		return fail("error parsing client peer list", helpers.WrapError(err, 2))
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
	for name, peer := range peers {
		wg.Add(1)
		go func(ctx context.Context, pname string, peer *Peer, out chan<- error) {

			logger := log.FromContext(ctx).With(zap.String("peer", pname), zap.String("as", auth.Name))
			syncd, err := client.New(ctx, peer.Address, client.WithDialOptions(dopt...))
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

			defer func() {
				if r := recover(); r != nil {
					logger.Error("recovered from panic")
					out <- helpers.NewError("panicked while syncing with "+pname, 1)
				}
				wg.Done()
			}()
			switch cmd.CalledAs() {
			case "push":
				if err := doPush(ctx, syncd, pname, peer, auth.Name, graphs); err != nil {
					out <- helpers.WrapError(err, 1)
				}
			case "pull":
				if err := doPull(ctx, syncd, peer, auth.Name, graphs); err != nil {
					out <- helpers.WrapError(err, 1)
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

func doPush(ctx context.Context, syncd client.Client, name string, peer *Peer, as string, graphs map[string]graph.Graph) (err error) {

	logger := log.FromContext(ctx)

	// Watch for a canceled context
	var done bool
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		<-ctx.Done()
		done = true
	}()

	for mname := range peer.PushModels {
		if done {
			logger.Info("push aborted due to canceled context")
			break
		}
		if g, ok := graphs[mname]; ok {
			var filters []graph.Filter
			for _, f := range peer.PushModels[mname].Filters {
				// TODO: filters need to vetted
				filters = append(filters, graph.Filter{
					Key:      f.Key,
					Operator: graph.FilterOperator(f.Operator),
					Value:    f.Values,
				})
			}
			if e := syncd.Push(ctx, mname, g.Source(name, filters...), as); e != nil {
				logger.Error("error pulling from peer", zap.Error(e))
				err = e
			}
		} else {
			logger.Error("unknown model", zap.String("model", mname))
			err = helpers.NewError("undefined model used in peer push configuration", 2)
		}
	}

	return
}

func doPull(ctx context.Context, syncd client.Client, peer *Peer, as string, graphs map[string]graph.Graph) (err error) {

	logger := log.FromContext(ctx)

	// Watch for a canceled context
	var done bool
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		<-ctx.Done()
		done = true
	}()

	// Pull all models
	for i := range peer.PullModels {
		if done {
			logger.Info("pull aborted due to canceled context")
			break
		}
		if g, ok := graphs[peer.PullModels[i]]; ok {
			if e := syncd.Pull(ctx, peer.PullModels[i], g.Destination(), as); e != nil {
				logger.Error("error pulling from peer", zap.Error(e))
				err = e
			}
		} else {
			logger.Error("unknown model", zap.String("model", peer.PullModels[i]))
			err = helpers.NewError("undefined model used in peer pull configuration", 2)
		}
	}

	return
}

type ClientAuth struct {
	Name         string
	PreSharedKey struct {
		FromEnv string
		Value   string
	}
	Peers []Peer
}

func (ca *ClientAuth) Validate() error {
	// TODO validate ClientAuth
	return nil
}

func (ca *ClientAuth) DialOptions() (opt []grpc.DialOption) {
	psk := ca.PreSharedKey.Value
	if e := ca.PreSharedKey.FromEnv; psk == "" && e != "" {
		psk = os.Getenv(e)
	}
	if psk != "" {
		opt = append(opt, client.WithPreSharedKey(psk))
	}
	return
}

type Peer struct {
	Address    string
	PullModels []string
	PushModels map[string]struct {
		Filters []struct {
			Key      string
			Operator string
			Values   []string
		}
	}
}
