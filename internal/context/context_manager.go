package context

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func CreateNotifyContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
}

func CreateContext() context.Context {
	return context.Background()
}

func CreateContextTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, timeout)
}
