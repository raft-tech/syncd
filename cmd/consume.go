package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

func NewConsume() *cobra.Command {
	return &cobra.Command{
		RunE:  Consume,
		Short: "Receive data from authenticated clients (see push)",
		Use:   "consume",
	}
}

func Consume(cmd *cobra.Command, args []string) (err error) {

	var config *viper.Viper
	if config, err = getConfig(cmd); err != nil {
		return
	}

	var logger *zap.Logger
	if logger, err = newLogger(cmd, config); err != nil {
		return
	}

	logger.Info("hello, consume!")

	return
}
