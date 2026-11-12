// Test-only Modbus TCP simulator. It is not embedded in or required by fastiot.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/simpleiot/simpleiot/modbus"
)

type config struct {
	Servers []serverConfig `json:"servers"`
}

type serverConfig struct {
	Port      int              `json:"port"`
	ID        int              `json:"id"`
	TimeoutMS int              `json:"timeout_ms"`
	Registers map[string][]int `json:"registers"`
	Coils     []int            `json:"coils"`
	Fault     faultConfig      `json:"fault"`
}

type faultConfig struct {
	DelayMS      int `json:"delay_ms"`
	CorruptEvery int `json:"corrupt_every"`
	DropAfter    int `json:"drop_after"`
}

type faultConn struct {
	net.Conn
	fault   faultConfig
	writes  atomic.Int64
	readErr bool
}

func (c *faultConn) Read(p []byte) (int, error) {
	if c.fault.DelayMS > 0 {
		time.Sleep(time.Duration(c.fault.DelayMS) * time.Millisecond)
	}
	if c.readErr {
		return 0, io.ErrUnexpectedEOF
	}
	return c.Conn.Read(p)
}

func (c *faultConn) Write(p []byte) (int, error) {
	if c.fault.DelayMS > 0 {
		time.Sleep(time.Duration(c.fault.DelayMS) * time.Millisecond)
	}
	count := c.writes.Add(1)
	if c.fault.DropAfter > 0 && count > int64(c.fault.DropAfter) {
		c.readErr = true
		_ = c.Conn.Close()
		return 0, io.ErrClosedPipe
	}
	if c.fault.CorruptEvery > 0 && count%int64(c.fault.CorruptEvery) == 0 && len(p) > 2 {
		copyBuf := append([]byte(nil), p...)
		copyBuf[len(copyBuf)-1] ^= 0xFF
		return c.Conn.Write(copyBuf)
	}
	return c.Conn.Write(p)
}

func main() {
	port := flag.Int("port", 1502, "listen port when no config is given")
	id := flag.Int("id", 1, "Modbus server ID when no config is given")
	value := flag.Int("value", 253, "register 10 initial value when no config is given")
	capacity := flag.Int("capacity", 2000, "register 11 capability value when no config is given")
	configPath := flag.String("config", "", "JSON fixture; when set, -port/-id/-value/-capacity are ignored")
	flag.Parse()

	if *configPath != "" {
		raw, err := os.ReadFile(*configPath)
		if err != nil {
			log.Fatal(err)
		}
		var cfg config
		if err := json.Unmarshal(raw, &cfg); err != nil {
			log.Fatalf("invalid config: %v", err)
		}
		if len(cfg.Servers) == 0 {
			log.Fatal("config has no servers")
		}
		for _, server := range cfg.Servers {
			go serve(server)
		}
		select {}
	}

	serve(serverConfig{
		Port: *port, ID: *id, TimeoutMS: 2000,
		Registers: map[string][]int{"0": {1}, "10": {*value, *capacity}},
	})
}

func serve(cfg serverConfig) {
	if cfg.Port == 0 {
		cfg.Port = 1502
	}
	if cfg.ID == 0 {
		cfg.ID = 1
	}
	if cfg.TimeoutMS == 0 {
		cfg.TimeoutMS = 2000
	}
	regs := &modbus.Regs{}
	for address, values := range cfg.Registers {
		start, err := strconv.Atoi(address)
		if err != nil || start < 0 {
			log.Fatalf("invalid register address %q", address)
		}
		regs.AddReg(start, len(values))
		for i, value := range values {
			if value < 0 || value > 65535 {
				log.Fatalf("register %d value %d out of range", start+i, value)
			}
			if err := regs.WriteReg(start+i, uint16(value)); err != nil {
				log.Fatalf("register %d: %v", start+i, err)
			}
		}
	}
	for _, coil := range cfg.Coils {
		regs.AddCoil(coil)
	}

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.Port))
	if err != nil {
		log.Fatalf("listen :%d: %v", cfg.Port, err)
	}
	log.Printf("TEST Modbus simulator listening on :%d id=%d registers=%d coils=%d fault=%+v",
		cfg.Port, cfg.ID, len(cfg.Registers), len(cfg.Coils), cfg.Fault)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("accept: %v", err)
			continue
		}
		go func(conn net.Conn) {
			wrapped := &faultConn{Conn: conn, fault: cfg.Fault}
			transport := modbus.NewTCP(wrapped, time.Duration(cfg.TimeoutMS)*time.Millisecond, modbus.TransportServer)
			server := modbus.NewServer(byte(cfg.ID), transport, regs, 0)
			server.Listen(func(err error) {
				if err != nil && err != io.EOF {
					log.Printf("connection: %v", err)
				}
				_ = conn.Close()
			}, func() {}, func() {})
		}(conn)
	}
}
