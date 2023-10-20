package cmd

import (
	"context"
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
	return &cobra.Command{
		RunE:  PushOrPull,
		Short: "Push data to remote peers",
		Use:   "push [FLAGS]",
	}
}

func NewPull() *cobra.Command {
	return &cobra.Command{
		RunE:  PushOrPull,
		Short: "Pull data from remote peers",
		Use:   "pull [FLAGS]",
	}
}

func PushOrPull(cmd *cobra.Command, args []string) (err error) {

	var config *viper.Viper
	if config, err = helpers.Config(cmd); err != nil {
		return
	}

	var logger *zap.Logger
	if logger, err = helpers.Logger(cmd.OutOrStdout(), config.Sub("logging")); err != nil {
		return
	}

	var auth ClientAuth
	if err = config.Unmarshal(&auth); err == nil {
		err = auth.Validate()
	}
	if err != nil {
		logger.Error("error parsing client auth", zap.Error(err))
		err = helpers.WrapError(err, 2)
		return
	}
	dopt := auth.DialOptions()

	var peers map[string]*Peer
	if err = config.Unmarshal(peers); err == nil {
		logger.Error("error parsing client peer list", zap.Error(err))
		err = helpers.WrapError(err, 2)
		return
	}

	var graphs map[string]graph.Graph
	var closer func(context.Context) error
	logger.Debug("initializing graph")
	if graphs, closer, err = helpers.Graph(cmd.Context(), config.Sub("graph")); err == nil {
		defer func() {
			if e := closer(cmd.Context()); e != nil {
				logger.Error("error closing graph factory", zap.Error(e))
			}
		}()
	} else {
		logger.Error("error initializing graph", zap.Error(err))
		return
	}
	logger.Info("graph initialized")

	errs := make(chan error)
	defer close(errs)
	wg := sync.WaitGroup{}
	for name, peer := range peers {
		wg.Add(1)
		go func(ctx context.Context, pname string, peer *Peer, out chan<- error) {

			logger := log.FromContext(ctx).With(zap.String("peer", pname), zap.String("as", auth.Name))
			var syncd client.Client
			syncd, err = client.New(ctx, peer.Address, client.WithDialOptions(dopt...))
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
	for e := range errs {
		err = e
	}
	wg.Wait()
	return
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
