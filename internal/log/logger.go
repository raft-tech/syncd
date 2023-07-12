package log

import (
	"errors"
	"fmt"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"io"
	"os"
	"strings"
)

type Options struct {
	Format string
	Level  zapcore.Level
	Out    io.Writer
}

var DefaultOptions = Options{
	Format: "json",
	Level:  zapcore.InfoLevel,
	Out:    os.Stdout,
}

func NewLoggerWithOptions(opt Options) (*zap.Logger, error) {
	var enc zapcore.Encoder
	switch e := strings.ToLower(opt.Format); e {
	case "":
		fallthrough
	case "text":
		enc = zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig())
	case "json":
		enc = zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	default:
		return nil, errors.New(fmt.Sprintf("unrecognized log format: %s", e))
	}
	return zap.New(zapcore.NewCore(enc, zapcore.AddSync(opt.Out), opt.Level)), nil
}
