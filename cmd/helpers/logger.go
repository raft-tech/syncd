/*
 * Copyright (c) 2023. Raft, LLC
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

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
	level := "DEBUG"
	if cfg != nil {
		if f := cfg.GetString("format"); f != "" {
			opt.Format = f
		}
		if l := cfg.GetString("level"); l != "" {
			level = strings.ToUpper(l)
		}
	}

	// Set log level
	switch level {
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
		return nil, NewError(fmt.Sprintf("unrecognized log level: %s", level), 2)
	}

	// Build logger from parsed options
	logger, err := log.NewLoggerWithOptions(opt)
	if err != nil {
		err = WrapError(err, 2)
	}

	return logger, err
}
