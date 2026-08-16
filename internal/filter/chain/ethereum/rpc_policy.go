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
