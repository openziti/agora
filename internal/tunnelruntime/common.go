package tunnelruntime

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
)

func proxyConnPair(left, right net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(left, right)
		_ = left.Close()
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(right, left)
		_ = right.Close()
	}()
	wg.Wait()
}

func closeOnDone(ctx context.Context, closer io.Closer) {
	go func() {
		<-ctx.Done()
		_ = closer.Close()
	}()
}

func ignoreClosedError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, net.ErrClosed) || errors.Is(err, http.ErrServerClosed) {
		if ctx.Err() != nil {
			return nil
		}
	}
	return err
}
