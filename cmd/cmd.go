package cmd

import (
	"context"
	"fmt"
	"github.com/raft-tech/syncd/internal/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
	"path"
	"strings"
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
		RunE:    root,
		Use:     "syncd",
		Version: Version,
	}
	cmd.PersistentFlags().String("config", "", "path/to/config.yaml")
	cmd.PersistentFlags().String("logging-level", "info", "one of: debug, info, warn, error, fatal")
	cmd.PersistentFlags().String("logging-format", "text", "one of: text, json")
	cmd.AddCommand(NewPublish(), NewPull(), NewConsume(), NewPush())
	return cmd
}

func root(cmd *cobra.Command, args []string) (err error) {
	return cmd.Help()
}

func getConfig(cmd *cobra.Command) (*viper.Viper, error) {

	if cmd == nil {
		panic("cmd must not be nil")
	}

	cfg := viper.New()

	// Support env variables prefixed with SYNCD_
	cfg.SetEnvPrefix("syncd")
	cfg.AutomaticEnv()

	// Load config from file
	configFile := os.Getenv("SYNCD_CONFIG")
	if c, _ := cmd.PersistentFlags().GetString("config"); c != "" {
		configFile = c
	}
	if configFile != "" {
		cfg.SetConfigFile(path.Clean(configFile))
		if err := cfg.ReadInConfig(); err != nil {
			return nil, WrapError(err, 2)
		}
	}

	// Bind root persistent flags
	if err := cfg.BindPFlag("logging.level", cmd.Flag("logging-level")); err != nil {
		return nil, WrapError(err, 2)
	}
	if err := cfg.BindPFlag("logging.format", cmd.Flag("logging-format")); err != nil {
		return nil, WrapError(err, 2)
	}

	return cfg, nil
}

func newLogger(cmd *cobra.Command, cfg *viper.Viper) (*zap.Logger, error) {

	opt := log.DefaultOptions
	opt.Out = cmd.OutOrStdout()
	opt.Format = cfg.GetString("logging.format") // Default is text, set by PersistentFlags on root command

	// Set log level
	switch l := strings.ToUpper(cfg.GetString("logging.level")); l {
	case "DEBUG":
		opt.Level = zapcore.DebugLevel
	case "INFO":
		opt.Level = zapcore.InfoLevel // Default, set by PersistentFlags on root command
	case "WARN":
		opt.Level = zapcore.WarnLevel
	case "ERROR":
		opt.Level = zapcore.ErrorLevel
	case "FATAL":
		opt.Level = zapcore.FatalLevel
	default:
		return nil, NewError(fmt.Sprintf("unrecognized log level: %s", l), 2)
	}

	// Build logger from parsed options
	logger, err := log.NewLoggerWithOptions(opt)
	if err != nil {
		err = WrapError(err, 2)
	}

	return logger, err
}
