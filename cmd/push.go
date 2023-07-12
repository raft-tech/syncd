package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

func NewPush() *cobra.Command {
	return &cobra.Command{
		RunE:  Push,
		Short: "Push data to configured consumers",
		Use:   "push",
	}
}

func Push(cmd *cobra.Command, args []string) (err error) {

	var config *viper.Viper
	if config, err = getConfig(cmd); err != nil {
		return
	}

	var logger *zap.Logger
	if logger, err = newLogger(cmd, config); err != nil {
		return
	}

	logger.Info("hello, push!")

	return
}
