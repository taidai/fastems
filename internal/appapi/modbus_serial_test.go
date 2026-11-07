package appapi

import (
	"testing"

	"github.com/simpleiot/simpleiot/data"
)

func TestNormalizeSerialConnection(t *testing.T) {
	input := ModbusConnectionInput{Protocol: data.PointValueRTU, Port: "COM3", Baud: "9600"}
	normalizeSerial(&input)
	if input.DataBits != 8 || input.Parity != "N" || input.StopBits != 1 {
		t.Fatalf("defaults = %+v", input)
	}
	input = ModbusConnectionInput{Protocol: data.PointValueRTU, Port: "COM3", Baud: "19200", DataBits: 7, Parity: "e", StopBits: 2}
	normalizeSerial(&input)
	if input.Parity != "E" || input.DataBits != 7 || input.StopBits != 2 {
		t.Fatalf("normalized = %+v", input)
	}
}

func TestSerialConnectionValidation(t *testing.T) {
	valid := ModbusConnectionInput{Protocol: data.PointValueRTU, Port: "COM3", Baud: "9600", DataBits: 8, Parity: "N", StopBits: 1}
	if err := validateModbusConnection(valid); err != nil {
		t.Fatalf("valid: %v", err)
	}
	for _, input := range []ModbusConnectionInput{
		{Protocol: data.PointValueRTU, Port: "COM3", Baud: "9600", DataBits: 9, Parity: "N", StopBits: 1},
		{Protocol: data.PointValueRTU, Port: "COM3", Baud: "9600", DataBits: 8, Parity: "X", StopBits: 1},
		{Protocol: data.PointValueRTU, Port: "COM3", Baud: "9600", DataBits: 8, Parity: "N", StopBits: 3},
	} {
		if err := validateModbusConnection(input); err == nil {
			t.Fatalf("expected error for %+v", input)
		}
	}
}

func TestSerialPoints(t *testing.T) {
	points := serialPoints(ModbusConnectionInput{DataBits: 7, Parity: "E", StopBits: 2})
	if len(points) != 3 || points[0].Type != "dataBits" || points[0].Val() != 7 {
		t.Fatalf("data bits = %+v", points[0])
	}
	if points[1].Type != "parity" || points[1].Txt() != "E" {
		t.Fatalf("parity = %+v", points[1])
	}
	if points[2].Type != "stopBits" || points[2].Val() != 2 {
		t.Fatalf("stop bits = %+v", points[2])
	}
}
