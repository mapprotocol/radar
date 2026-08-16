package ethereum

import (
	"context"
	"errors"
	"net"
	"time"
)

const (
	latestBlockRPCTimeout    = 3 * time.Second
	blockHeaderRPCTimeout    = 5 * time.Second
	filterLogsRPCTimeout     = 10 * time.Second
	historicalLogsRPCTimeout = 30 * time.Second
)

type idleConnectionCloser interface {
	CloseIdleConnections()
}

func runRPCCall[T any](
	parent context.Context,
	timeout time.Duration,
	connection any,
	call func(context.Context) (T, error),
) (T, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	result, err := call(ctx)
	ctxErr := ctx.Err()
	cancel()
	closeIdleConnectionsOnRPCError(connection, ctxErr, err)
	return result, err
}

func closeIdleConnectionsOnRPCError(connection any, ctxErr, err error) {
	if err == nil {
		return
	}

	var networkError net.Error
	if !errors.Is(ctxErr, context.DeadlineExceeded) &&
		!errors.Is(err, context.DeadlineExceeded) &&
		!errors.As(err, &networkError) {
		return
	}

	if closer, ok := connection.(idleConnectionCloser); ok {
		closer.CloseIdleConnections()
	}
}
