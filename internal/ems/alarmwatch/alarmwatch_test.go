package alarmwatch

import (
	"strings"
	"testing"
	"time"

	"easyems/internal/ems/alarm"
)

var now = time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)

func view(id, typ string) NodeView {
	return NodeView{ID: id, Type: typ, Text: map[string]string{}, Num: map[string]float64{}}
}

func find(findings []Finding, key string) (Finding, bool) {
	for _, finding := range findings {
		if finding.Key == key {
			return finding, true
		}
	}
	return Finding{}, false
}

func TestRuleErrorRaisesCritical(t *testing.T) {
	node := view("rule1", "decisionTable")
	node.Text["error"] = "target unavailable"
	findings := Evaluate([]NodeView{node}, now)
	finding, ok := find(findings, "rule-error:rule1")
	if !ok || finding.Severity != alarm.Critical || !strings.Contains(finding.Detail, "target") {
		t.Fatalf("findings = %+v", findings)
	}
}

func TestBusHeartbeat(t *testing.T) {
	bus := view("bus1", "modbus")
	bus.Num["pollPeriod"] = 1000
	bus.Num["timeout"] = 1000
	bus.Num["lastGoodPoll"] = float64(now.Add(-time.Second).Unix())
	if findings := Evaluate([]NodeView{bus}, now); len(findings) != 0 {
		t.Fatalf("fresh bus raised %+v", findings)
	}

	bus.Num["lastGoodPoll"] = float64(now.Add(-60 * time.Second).Unix())
	findings := Evaluate([]NodeView{bus}, now)
	finding, ok := find(findings, "bus-offline:bus1")
	if !ok {
		t.Fatalf("stale bus not raised: %+v", findings)
	}
	if finding.Severity != alarm.Critical {
		t.Fatalf("severity = %s", finding.Severity)
	}
}

func TestNewBusGetsGrace(t *testing.T) {
	bus := view("bus1", "modbus")
	bus.Created = now.Add(-time.Second)
	bus.Num["pollPeriod"] = 1000
	bus.Num["timeout"] = 1000
	if findings := Evaluate([]NodeView{bus}, now); len(findings) != 0 {
		t.Fatalf("new bus raised %+v", findings)
	}
}

func TestDisabledBusIgnored(t *testing.T) {
	bus := view("bus1", "modbus")
	bus.Num["disabled"] = 1
	bus.Num["lastGoodPoll"] = float64(now.Add(-time.Hour).Unix())
	if findings := Evaluate([]NodeView{bus}, now); len(findings) != 0 {
		t.Fatalf("disabled bus raised %+v", findings)
	}
}

func TestCommandFailureRaised(t *testing.T) {
	point := view("io1", "modbusIo")
	point.Text["lastCommand"] = `{"status":"unknown"}`
	findings := Evaluate([]NodeView{point}, now)
	finding, ok := find(findings, "command-unknown:io1")
	if !ok || finding.Severity != alarm.Warning {
		t.Fatalf("findings = %+v", findings)
	}

	point.Text["lastCommand"] = `{"status":"confirmed"}`
	if findings := Evaluate([]NodeView{point}, now); len(findings) != 0 {
		t.Fatalf("confirmed command raised %+v", findings)
	}
}
