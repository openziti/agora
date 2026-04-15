package tunnelruntime

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

func proxyConnPair(left, right net.Conn) {
	proxyConnPairWithStats(left, right, nil)
}

type proxyConnStats struct {
	LeftToRightBytes int64
	RightToLeftBytes int64
	LeftToRightErr   error
	RightToLeftErr   error
	Duration         time.Duration
}

type proxyCopyResult struct {
	bytes int64
	err   error
}

func proxyConnPairWithStats(left, right net.Conn, onComplete func(proxyConnStats)) {
	startedAt := time.Now()
	var wg sync.WaitGroup
	wg.Add(2)
	rightToLeft := make(chan proxyCopyResult, 1)
	leftToRight := make(chan proxyCopyResult, 1)
	go func() {
		defer wg.Done()
		n, err := io.Copy(left, right)
		rightToLeft <- proxyCopyResult{bytes: n, err: err}
		_ = left.Close()
	}()
	go func() {
		defer wg.Done()
		n, err := io.Copy(right, left)
		leftToRight <- proxyCopyResult{bytes: n, err: err}
		_ = right.Close()
	}()
	wg.Wait()

	if onComplete != nil {
		leftToRightResult := <-leftToRight
		rightToLeftResult := <-rightToLeft
		stats := proxyConnStats{
			LeftToRightBytes: leftToRightResult.bytes,
			RightToLeftBytes: rightToLeftResult.bytes,
			LeftToRightErr:   leftToRightResult.err,
			RightToLeftErr:   rightToLeftResult.err,
			Duration:         time.Since(startedAt),
		}
		onComplete(stats)
	}
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
