package cmd

import (
	"context"

	"github.com/spf13/cobra"
	"go.uber.org/zap/zapcore"
)

var Version string

var DefaultOptions = Options{
	LogLevel: zapcore.WarnLevel,
}

type Options struct {
	ConfigFile string
	Context    context.Context
	LogLevel   zapcore.Level
}

func New(opt Options) *cobra.Command {
	cmd := &cobra.Command{
		SilenceErrors: true,
		SilenceUsage:  true,
		Use:           "syncd [FLAGS] COMMAND",
		Version:       Version,
	}
	cmd.PersistentFlags().StringSlice("config", nil, "path/to/config.yaml")
	cmd.PersistentFlags().String("logging.level", "info", "one of: debug, info, warn, error, fatal")
	cmd.PersistentFlags().String("logging.format", "text", "one of: text, json")
	cmd.AddCommand(NewPush(), NewPull(), NewServer())
	return cmd
}
