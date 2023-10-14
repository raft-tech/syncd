package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

func NewPull() *cobra.Command {
	return &cobra.Command{
		RunE:  Pull,
		Short: "Pull data from configured publishers",
		Use:   "pull",
	}
}

func Pull(cmd *cobra.Command, _ []string) (err error) {

	var config *viper.Viper
	if config, err = getConfig(cmd); err != nil {
		return
	}

	var logger *zap.Logger
	if logger, err = newLogger(cmd, config); err != nil {
		return
	}

	logger.Info("hello, pull!")

	return
}
