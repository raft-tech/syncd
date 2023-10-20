package helpers

import (
	"fmt"
	"io"
	"strings"

	"github.com/raft-tech/syncd/internal/log"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func Logger(writer io.Writer, cfg *viper.Viper) (*zap.Logger, error) {

	opt := log.DefaultOptions
	opt.Out = writer
	opt.Format = cfg.GetString("format") // Default is text, set by PersistentFlags on root command

	// Set log level
	switch l := strings.ToUpper(cfg.GetString("level")); l {
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
