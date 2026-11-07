// Package ems is the Encore service entrypoint for easyems.
//
// Encore owns the process, public HTTP listener, tracing and deployment. The
// embedded easyems handler serves the UI and product API in-process; no
// second listener or reverse proxy is involved.
package ems

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"easyems/internal/app"
)

//encore:service
type Service struct {
	app *app.App
}

func initService() (*Service, error) {
	dataDir := os.Getenv("EASYEMS_DATA")
	if dataDir == "" {
		dataDir = "data"
	}
	version := os.Getenv("EASYEMS_VERSION")
	if version == "" {
		version = "development"
	}

	natsPort, err := app.FreePort()
	if err != nil {
		return nil, fmt.Errorf("reserve easyems NATS port: %w", err)
	}

	application, err := app.Start(context.Background(), app.Config{
		DataDir:     dataDir,
		NATSPort:    natsPort,
		Version:     version,
		DisableHTTP: true,
	})
	if err != nil {
		return nil, err
	}
	return &Service{app: application}, nil
}

// Fallback serves the embedded UI and any path not claimed by a named route.
//
//encore:api public raw path=/!fallback
func (s *Service) Fallback(w http.ResponseWriter, req *http.Request) {
	s.serve(w, req)
}

// Health is an Encore-native read-only route.
//
//encore:api public raw method=GET path=/api/health
func (s *Service) Health(w http.ResponseWriter, req *http.Request) {
	s.serve(w, req)
}

// Login is an Encore-native authentication route.
//
//encore:api public raw method=POST path=/api/login
func (s *Service) Login(w http.ResponseWriter, req *http.Request) {
	s.serve(w, req)
}

// DeviceLedger is an Encore-native read-only route.
//
//encore:api public raw method=GET path=/api/device-ledger
func (s *Service) DeviceLedger(w http.ResponseWriter, req *http.Request) {
	s.serve(w, req)
}

// AlarmStats is an Encore-native read-only route.
//
//encore:api public raw method=GET path=/api/alarm-stats
func (s *Service) AlarmStats(w http.ResponseWriter, req *http.Request) {
	s.serve(w, req)
}

func (s *Service) serve(w http.ResponseWriter, req *http.Request) {
	if s.app == nil {
		http.Error(w, "easyems backend not initialized", http.StatusServiceUnavailable)
		return
	}
	s.app.ServeHTTP(w, req)
}

// Shutdown stops the embedded simpleiot/EMS runtime.
func (s *Service) Shutdown(force context.Context) {
	if s.app == nil {
		return
	}
	done := make(chan struct{})
	go func() {
		if err := s.app.Close(); err != nil {
			log.Printf("easyems shutdown: %v", err)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-force.Done():
		log.Print("easyems shutdown: forced before embedded runtime exited")
	}
}
