package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/controller/config"
	"github.com/openziti/agora/internal/persistence"
	"github.com/openziti/agora/ui"
)

const (
	shutdownDrainTimeout   = 15 * time.Second
	readinessProbeTimeout  = 2 * time.Second
	defaultHTTPReadHeader  = 10 * time.Second
	defaultHTTPIdleTimeout = 120 * time.Second
)

type Controller struct {
	cfg     *config.Config
	store   *persistence.Store
	service *Service
}

func New(cfg *config.Config) (*Controller, error) {
	dl.Infof("opening persistence store for controller bind_address='%s'", cfg.BindAddress)
	store, err := persistence.Open(context.Background(), cfg.Store)
	if err != nil {
		return nil, err
	}
	dl.Infof("opened persistence store for controller bind_address='%s'", cfg.BindAddress)
	service := NewService(cfg, store)
	return &Controller{
		cfg:     cfg,
		store:   store,
		service: service,
	}, nil
}

func Run(cfg *config.Config) error {
	dl.Infof("starting agora controller bind_address='%s'", cfg.BindAddress)
	controller, err := New(cfg)
	if err != nil {
		return fmt.Errorf("create controller: %w", err)
	}
	defer func() {
		if err := controller.store.Close(); err != nil {
			dl.Errorf("error closing controller store: %v", err)
		}
	}()

	dl.Info("checking schema compatibility")
	if err := persistence.CheckSchemaCompatibility(context.Background(), controller.store); err != nil {
		return fmt.Errorf("check schema compatibility: %w", err)
	}
	dl.Info("schema compatibility check passed")

	dl.Info("building controller http handler")
	handler, err := newControllerHTTPHandler(controller)
	if err != nil {
		return fmt.Errorf("build handler: %w", err)
	}

	rootCtx, cancelRoot := context.WithCancel(context.Background())
	defer cancelRoot()

	var reapers sync.WaitGroup
	reapers.Add(3)
	go func() {
		defer reapers.Done()
		controller.service.RunTunnelAttachmentReaper(rootCtx)
	}()
	go func() {
		defer reapers.Done()
		controller.service.RunTunnelServeReaper(rootCtx)
	}()
	go func() {
		defer reapers.Done()
		controller.service.RunSessionDurationReaper(rootCtx)
	}()

	srv := &http.Server{
		Addr:              controller.cfg.BindAddress,
		Handler:           handler,
		ReadHeaderTimeout: defaultHTTPReadHeader,
		IdleTimeout:       defaultHTTPIdleTimeout,
	}

	signalCtx, stopSignal := signal.NotifyContext(rootCtx, os.Interrupt, syscall.SIGTERM)
	defer stopSignal()

	serveErrCh := make(chan error, 1)
	go func() {
		dl.Infof("controller http handler ready; listening on '%s'", controller.cfg.BindAddress)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErrCh <- err
			return
		}
		serveErrCh <- nil
	}()

	select {
	case err := <-serveErrCh:
		cancelRoot()
		reapers.Wait()
		if err != nil {
			return fmt.Errorf("http listen: %w", err)
		}
		return nil
	case <-signalCtx.Done():
		dl.Info("shutdown signal received; draining http server")
	}

	drainCtx, cancelDrain := context.WithTimeout(context.Background(), shutdownDrainTimeout)
	defer cancelDrain()
	if err := srv.Shutdown(drainCtx); err != nil {
		dl.Errorf("http shutdown error: %v", err)
	} else {
		dl.Info("http server drained cleanly")
	}

	cancelRoot()
	reapers.Wait()
	dl.Info("controller shutdown complete")

	if err := <-serveErrCh; err != nil {
		return fmt.Errorf("http listen: %w", err)
	}
	return nil
}

func newControllerHTTPHandler(controller *Controller) (http.Handler, error) {
	apiHandler, err := NewHandler(controller.service)
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.Handle("/", ui.Middleware(authMiddlewareStack(controller.service, authCookieSecure(controller.cfg), apiHandler)))
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/ready", readinessHandler(controller.store))
	return mux, nil
}

func authMiddlewareStack(svc *Service, secureCookies bool, apiHandler http.Handler) http.Handler {
	handler := logoutCookieClearMiddleware(secureCookies)(apiHandler)
	handler = loginCookieEmitMiddleware(secureCookies)(handler)
	handler = csrfMiddleware(csrfSkipPaths)(handler)
	handler = principalAttachMiddleware(svc, optionalPrincipalAttachPaths)(handler)
	handler = cookieToHeaderMiddleware(handler)
	return handler
}

func authCookieSecure(_ *config.Config) bool {
	return false
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func readinessHandler(store *persistence.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), readinessProbeTimeout)
		defer cancel()
		if err := store.DB().PingContext(ctx); err != nil {
			dl.Warnf("readiness probe failed: %v", err)
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("not ready"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	}
}
