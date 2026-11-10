// Package app owns the easyems process lifecycle: Simple IoT, NATS/JetStream,
// the EMS runtime, alarm watcher, bridge, and HTTP API.
//
// It is shared by the legacy single-binary command and the Encore service.
// The caller decides who owns signals and process lifecycle.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/oklog/run"
	"github.com/simpleiot/simpleiot/api"
	"github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"
	"github.com/simpleiot/simpleiot/server"

	"easyems/internal/appapi"
	"easyems/internal/audit"
	"easyems/internal/decision"
	"easyems/internal/ems/alarmwatch"
	"easyems/internal/ems/bridge"
	"easyems/internal/ems/channel"
	"easyems/internal/ems/modbusrt"
	"easyems/internal/ems/profile"
	"easyems/internal/ems/runtime"
	"easyems/internal/emsapi"
	"easyems/internal/web"
)

// Config describes one easyems instance.
type Config struct {
	DataDir     string
	HTTPPort    string
	NATSPort    int
	Version     string
	DisableHTTP bool
}

// App is a running easyems instance. The embedding runtime owns the public
// listener; ServeHTTP serves the UI and the product API in-process.
type App struct {
	cancel  context.CancelFunc
	done    chan error
	exited  chan struct{}
	handler *atomic.Value

	mu      sync.Mutex
	doneErr error
}

// FreePort reserves and releases a loopback TCP port.
func FreePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func envMinutes(name string, def int) time.Duration {
	if text := os.Getenv(name); text != "" {
		if minutes, err := strconv.Atoi(text); err == nil && minutes > 0 {
			return time.Duration(minutes) * time.Minute
		}
	}
	return time.Duration(def) * time.Minute
}

func envCount(name string, def int) int {
	if text := os.Getenv(name); text != "" {
		if count, err := strconv.Atoi(text); err == nil && count > 0 {
			return count
		}
	}
	return def
}

// Start boots easyems and returns once the HTTP listener is ready.
// Closing the returned App (or canceling ctx) shuts it down.
func Start(ctx context.Context, cfg Config) (*App, error) {
	if cfg.DataDir == "" {
		return nil, errors.New("easyems: data directory is required")
	}
	if cfg.HTTPPort == "" && !cfg.DisableHTTP {
		return nil, errors.New("easyems: HTTP port is required")
	}
	if cfg.NATSPort == 0 {
		return nil, errors.New("easyems: NATS port is required")
	}
	if cfg.Version == "" {
		cfg.Version = "development"
	}

	runCtx, runCancel := context.WithCancel(ctx)
	var closeOnce sync.Once
	closeApp := func() { closeOnce.Do(runCancel) }

	ready := new(atomic.Bool)
	historyLimit := appapi.ReadHistoryLimit(cfg.DataDir)
	emsService := runtime.NewService(nil, 15*time.Second, 5*time.Second, historyLimit)
	appapi.LoadAlarmMaintenance(cfg.DataDir, emsService.Alarms)
	appapi.WireCommandHistory(cfg.DataDir, emsService.Commands())
	emsHandler := emsapi.New(emsService, 15*time.Second)
	emsHandler.Profiles = profile.Load(filepath.Join(cfg.DataDir, "profiles"))
	for _, message := range emsHandler.Profiles.LoadErrors {
		log.Println("easyems profiles:", message)
	}
	handlerRef := new(atomic.Value)
	var energyDispatch func(string) error
	var reconcileCabinet func()
	var forecastTick func(time.Time)
	var opcuaManager *appapi.OpcuaManager
	options := server.Options{
		ID:           "easyems",
		DataDir:      cfg.DataDir,
		HTTPPort:     cfg.HTTPPort,
		DisableHTTP:  cfg.DisableHTTP,
		NatsPort:     cfg.NATSPort,
		NatsServer:   fmt.Sprintf("nats://127.0.0.1:%d", cfg.NATSPort),
		AuthToken:    data.NewID(),
		DeviceAuth:   server.DeviceAuthRequired,
		CustomUIFS:   web.FS(),
		AppOnly:      true,
		NatsLoopback: true,
		AppHandlerFactory: func(nc *nats.Conn, users api.UserAuthority) http.Handler {
			handler := appapi.New(nc, users, ready)
			handler.SetAudit(audit.New(filepath.Join(cfg.DataDir, "audit.jsonl")))
			handler.SetDataDir(cfg.DataDir)
			handler.SetSessionPolicy(envMinutes("EASYEMS_SESSION_TIMEOUT_MINUTES", 30), envCount("EASYEMS_MAX_SESSIONS", 5))
			emsHandler.PersistMaintenance = handler.PersistAlarmMaintenance
			handler.SetEMS(emsHandler)
			energyDispatch = func(id string) error {
				return handler.RunUnitArbitratedDispatch(id)
			}
			reconcileCabinet = handler.ReconcileCabinetCommands
			forecastTick = handler.RunForecastTick
			opcuaManager = appapi.NewOpcuaManager(nc, emsService)
			handler.SetOpcua(opcuaManager)
			fullHandler := api.NewAppHandler(api.ServerArgs{
				AppOnly:      true,
				AppHandler:   handler,
				UIFilesystem: web.FS(),
			})
			handlerRef.Store(http.Handler(fullHandler))
			return handler
		},
		ProvisioningDisable:    true,
		StoreSyncAlways:        true,
		AppVersion:             cfg.Version,
		StoreMaxMsgsPerSubject: int64(historyLimit),
	}

	siot, nc, err := server.NewServer(options)
	if err != nil {
		return nil, err
	}
	clients := client.NewRunGroup("easyems clients")
	clients.Add(client.NewManager(nc, decision.NewDecisionTableClient, nil))
	// 多协议接入：注册 SimpleIoT 的 MQTT 客户端，让 MQTT 设备可接入节点树。
	// Modbus 走自建 bridge，MQTT 复用上游 client。CAN 需硬件，留作可选。
	clients.Add(client.NewManager(nc, client.NewMqttClient, nil))
	siot.AddClient(clients)

	var group run.Group
	group.Add(siot.Run, siot.Stop)

	emsCtx, emsCancel := context.WithCancel(runCtx)
	group.Add(func() error {
		if err := emsService.Run(emsCtx); err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
		return nil
	}, func(error) {
		emsCancel()
		emsService.Close()
	})

	energyScheduler := appapi.NewEnergyScheduler(nc, func(energyUnitID string) error {
		if energyDispatch == nil {
			return nil
		}
		return energyDispatch(energyUnitID)
	})
	energyScheduler.SetReconcile(func() {
		if reconcileCabinet != nil {
			reconcileCabinet()
		}
	})
	energyScheduler.SetForecast(func(now time.Time) {
		if forecastTick != nil {
			forecastTick(now)
		}
	})
	group.Add(energyScheduler.Run, energyScheduler.Stop)

	watcher := alarmwatch.NewWatcher(nc, emsService.Alarms)
	group.Add(func() error {
		return watcher.Run(emsCtx)
	}, func(error) {
		emsCancel()
	})

	nodeSource := bridge.NewNodeSource(nc)
	emsHandler.Buses = nodeSource.Buses
	emsHandler.Configure = appapi.NewChannelAdmin(nc)
	nodeBridge := bridge.New(emsService, nodeSource, openEMSChannel)
	nodeBridge.Log = func(err error) { log.Println("ems bridge:", err) }
	emsHandler.RestartChannel = nodeBridge.Restart
	group.Add(func() error {
		return nodeBridge.Run(emsCtx)
	}, func(error) {
		emsCancel()
	})

	readyCh := make(chan struct{})
	var readyOnce sync.Once
	startupErr := make(chan error, 1)
	reportStartup := func(err error) error {
		select {
		case startupErr <- err:
		default:
		}
		return err
	}

	startCtx, startCancel := context.WithTimeout(runCtx, 10*time.Second)
	defer startCancel()
	group.Add(func() error {
		if err := siot.WaitStart(startCtx); err != nil {
			return reportStartup(err)
		}
		if err := appapi.Bootstrap(nc, cfg.DataDir); err != nil {
			return reportStartup(err)
		}
		appapi.ApplyRuntimeSettings(cfg.DataDir, emsService)
		ready.Store(true)
		if !cfg.DisableHTTP {
			if err := verifyHTTP(cfg.HTTPPort); err != nil {
				return reportStartup(err)
			}
		}
		log.Printf("easyems %s ready", cfg.Version)
		readyOnce.Do(func() { close(readyCh) })

		// 演示站点只在测试/本地显式启用，生产新实例默认不生成模拟资产。
		if os.Getenv("EASYEMS_SEED_DEMO") == "1" {
			go func() {
				time.Sleep(3 * time.Second)
				if err := appapi.SeedDemoSite(nc); err != nil {
					log.Printf("easyems: seed demo site: %v", err)
				}
			}()
		}

		<-runCtx.Done()
		return nil
	}, func(error) {
		startCancel()
		readyOnce.Do(func() { close(readyCh) })
	})

	// An external cancellation (context or App.Close) stops the group.
	group.Add(func() error {
		<-runCtx.Done()
		return nil
	}, func(error) {})

	done := make(chan error, 1)
	go func() { done <- group.Run() }()

	application := &App{
		cancel:  closeApp,
		done:    done,
		exited:  make(chan struct{}),
		handler: handlerRef,
	}
	go func() {
		err := <-done
		application.mu.Lock()
		application.doneErr = err
		application.mu.Unlock()
		close(application.exited)
	}()

	select {
	case <-readyCh:
		return application, nil
	case err := <-startupErr:
		closeApp()
		<-application.exited
		return nil, err
	case <-application.exited:
		err := application.Err()
		if err == nil {
			err = context.Canceled
		}
		return nil, err
	case <-ctx.Done():
		closeApp()
		<-application.exited
		return nil, ctx.Err()
	}
}

// Exited is closed when the underlying run group stops.
func (a *App) Exited() <-chan struct{} { return a.exited }

// ServeHTTP serves the embedded UI and product API without a network hop.
// The handler owns /api routing, authentication, scoping and response shape.
func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if a.handler == nil {
		http.Error(w, "easyems backend not initialized", http.StatusServiceUnavailable)
		return
	}
	handler, _ := a.handler.Load().(http.Handler)
	if handler == nil {
		http.Error(w, "easyems backend not initialized", http.StatusServiceUnavailable)
		return
	}
	handler.ServeHTTP(w, r)
}

// Err reports why the instance stopped. It is valid after Exited is closed.
func (a *App) Err() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	err := a.doneErr
	if errors.Is(err, server.ErrServerStopped) {
		return nil
	}
	return err
}

// Close stops easyems and waits for its subsystems to exit.
func (a *App) Close() error {
	a.cancel()
	<-a.exited
	return a.Err()
}

func verifyHTTP(port string) error {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		response, err := client.Get(fmt.Sprintf("http://127.0.0.1:%s/api/health", port))
		if err == nil {
			var health struct {
				Ready   bool   `json:"ready"`
				Version string `json:"version"`
			}
			err = json.NewDecoder(response.Body).Decode(&health)
			response.Body.Close()
			if err == nil && health.Ready && health.Version == "0.1.0" {
				return nil
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("easyems HTTP listener did not become ready on port %s", port)
}

// openEMSChannel opens a transport for a channel described by persisted nodes.
func openEMSChannel(protocol, address string, timeout time.Duration) (channel.Bus, error) {
	switch protocol {
	case "tcp", "":
		return modbusrt.OpenTCP(address, timeout)
	case "rtu":
		cfg, err := modbusrt.ParseSerialAddress(address)
		if err != nil {
			return nil, err
		}
		return modbusrt.OpenRTUConfig(cfg, timeout)
	default:
		return nil, fmt.Errorf("unsupported protocol %q", protocol)
	}
}
