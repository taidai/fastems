package modbusrt

import (
	"fastiot/internal/ems/framelog"

	"github.com/simpleiot/simpleiot/modbus"
)

type recordingTransport struct {
	modbus.Transport
	peer string
}

func (t *recordingTransport) Read(p []byte) (int, error) {
	n, err := t.Transport.Read(p)
	if n > 0 {
		framelog.Default.Record(t.peer, "rx", append([]byte(nil), p[:n]...))
	}
	return n, err
}

func (t *recordingTransport) Write(p []byte) (int, error) {
	n, err := t.Transport.Write(p)
	if n > 0 {
		framelog.Default.Record(t.peer, "tx", append([]byte(nil), p[:n]...))
	}
	return n, err
}
