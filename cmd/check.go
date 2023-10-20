package cmd

import (
	"github.com/raft-tech/syncd/cmd/helpers"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

func NewChecker() *cobra.Command {
	return &cobra.Command{
		RunE:  Check,
		Short: "Check remote peers",
		Use:   "check",
	}
}

func Check(cmd *cobra.Command, args []string) (err error) {

	var config *viper.Viper
	if config, err = helpers.Config(cmd); err != nil {
		return
	}

	var logger *zap.Logger
	if logger, err = helpers.Logger(cmd.OutOrStdout(), config.Sub("logging")); err != nil {
		return
	}

	logger.Info("hello, check!")

	return
}
