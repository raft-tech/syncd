package log

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type loggerKey string

const ctxKey = "logger"

type Options struct {
	Format string
	Level  zapcore.Level
	Out    io.Writer
}

var nop = zap.NewNop()

var DefaultOptions = Options{
	Format: "text",
	Level:  zapcore.InfoLevel,
	Out:    os.Stdout,
}

func NewLoggerWithOptions(opt Options) (*zap.Logger, error) {
	var zopt []zap.Option
	var enc zapcore.Encoder
	switch e := strings.ToLower(opt.Format); e {
	case "":
		fallthrough
	case "text":
		enc = zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig())
		zopt = append(zopt, zap.AddStacktrace(zap.ErrorLevel))
	case "json":
		enc = zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	default:
		return nil, errors.New(fmt.Sprintf("unrecognized log format: %s", e))
	}
	return zap.New(zapcore.NewCore(enc, zapcore.AddSync(opt.Out), opt.Level), zopt...), nil
}

func NewContext(ctx context.Context, l *zap.Logger) context.Context {
	return context.WithValue(ctx, ctxKey, l)
}

func FromContext(ctx context.Context) *zap.Logger {
	if l, ok := ctx.Value(ctxKey).(*zap.Logger); ok {
		return l
	} else {
		return nop
	}
}
