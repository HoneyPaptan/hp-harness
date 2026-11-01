package transport

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

func Failure(ctx context.Context, err error) error {
	if cause := ctx.Err(); cause != nil {
		return cause
	}
	return &provider.TransientError{Err: err}
}

func ReadFailure(ctx context.Context, err error) error {
	if errors.Is(err, bufio.ErrTooLong) {
		return err
	}
	return Failure(ctx, err)
}

func DecodeFailure(ctx context.Context, data string, err error) error {
	var syntax *json.SyntaxError
	if errors.As(err, &syntax) && syntax.Offset >= int64(len(data)) {
		return Failure(ctx, err)
	}
	return err
}
