package appapi

import (
	"net"
	"strconv"
	"testing"

	"github.com/simpleiot/simpleiot/data"
	siotmodbus "github.com/simpleiot/simpleiot/modbus"
)

func TestModbusConnectionFromNode(t *testing.T) {
	tcp := data.NodeEdge{
		Type: data.NodeTypeModbus,
		Points: data.Points{
			data.NewPointString(data.PointTypeProtocol, "0", data.PointValueTCP),
			data.NewPointString(data.PointTypeURI, "0", "127.0.0.1:1502"),
			data.NewPointFloat(data.PointTypePollPeriod, "0", 1000),
			data.NewPointFloat(data.PointTypeTimeout, "0", 500),
		},
	}
	conn, ok := modbusConnectionFromNode(tcp)
	if !ok || conn.Protocol != data.PointValueTCP || conn.URI != "127.0.0.1:1502" {
		t.Fatalf("tcp = %+v ok=%v", conn, ok)
	}

	rtu := data.NodeEdge{
		Type: data.NodeTypeModbus,
		Points: data.Points{
			data.NewPointString(data.PointTypeProtocol, "0", data.PointValueRTU),
			data.NewPointString(data.PointTypePort, "0", "COM3"),
			data.NewPointString(data.PointTypeBaud, "0", "19200"),
			data.NewPointFloat("dataBits", "0", 7),
			data.NewPointString("parity", "0", "E"),
			data.NewPointFloat("stopBits", "0", 2),
		},
	}
	conn, ok = modbusConnectionFromNode(rtu)
	if !ok || conn.Protocol != data.PointValueRTU || conn.Port != "COM3" || conn.Baud != "19200" {
		t.Fatalf("rtu = %+v ok=%v", conn, ok)
	}
	if conn.DataBits != 7 || conn.Parity != "E" || conn.StopBits != 2 {
		t.Fatalf("rtu framing = %+v", conn)
	}
}

func TestProbeLineTCP(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	regs := &siotmodbus.Regs{}
	regs.AddReg(0, 1)
	server, err := siotmodbus.NewTCPServer(1, 5, strconv.Itoa(port), regs, 0)
	if err != nil {
		t.Fatal(err)
	}
	serverDone := make(chan struct{})
	go server.Listen(func(error) {}, func() {}, func() { close(serverDone) })
	defer func() {
		_ = server.Close()
		<-serverDone
	}()

	result := probeLine(ModbusConnectionInput{
		Protocol: data.PointValueTCP,
		URI:      "127.0.0.1:" + strconv.Itoa(port),
	}, 1, 2)
	if !result.Connected {
		t.Fatalf("expected connected, got %+v", result)
	}
	if len(result.Stations) != 2 {
		t.Fatalf("expected 2 stations, got %d", len(result.Stations))
	}
	if !result.Stations[0].Responded {
		t.Fatalf("station 1 should respond: %+v", result.Stations[0])
	}
	if result.Stations[1].Responded {
		t.Fatalf("station 2 should not respond: %+v", result.Stations[1])
	}
}

func TestProbeLineUnreachable(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	result := probeLine(ModbusConnectionInput{
		Protocol: data.PointValueTCP,
		URI:      "127.0.0.1:" + strconv.Itoa(port),
	}, 1, 2)
	if result.Connected {
		t.Fatalf("expected connection failure, got %+v", result)
	}
	if len(result.Stations) != 2 || result.Stations[0].Error == "" {
		t.Fatalf("expected per-station error: %+v", result)
	}
}
