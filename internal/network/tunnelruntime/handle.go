package tunnelruntime

type Handle struct {
	done chan error
}

func newHandle(run func() error) *Handle {
	done := make(chan error, 1)
	go func() {
		defer close(done)
		done <- run()
	}()
	return &Handle{done: done}
}

func (h *Handle) Done() <-chan error {
	return h.done
}
