package audit

import (
	"bytes"
	"encoding/csv"
	"path/filepath"
	"testing"
	"time"
)

func TestAppendQueryAndCSV(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "audit.jsonl"))
	now := time.Now()
	records := []Record{
		{Time: now.Add(-time.Minute), Actor: "admin", Action: "登录", Result: "success", IP: "127.0.0.1"},
		{Time: now, Actor: "admin", Action: "命令执行", Object: "/api/modbus/a/units/1/commands", Result: "success"},
		{Time: now, Actor: "viewer", Action: "命令执行", Result: "failure"},
	}
	for _, record := range records {
		if err := store.Append(record); err != nil {
			t.Fatal(err)
		}
	}
	found, err := store.Query(Filter{Action: "命令执行"})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 2 || found[0].Actor != "viewer" {
		t.Fatalf("query = %+v", found)
	}
	found, _ = store.Query(Filter{Actor: "admin", Result: "success"})
	if len(found) != 2 {
		t.Fatalf("admin success = %+v", found)
	}
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := store.WriteCSV(writer, found); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("命令执行")) || !bytes.Contains(buf.Bytes(), []byte("操作人")) {
		t.Fatalf("csv = %s", buf.String())
	}
}

func TestParseRange(t *testing.T) {
	from, err := ParseRange("2026-11-06", false)
	if err != nil || from.Hour() != 0 || from.Minute() != 0 {
		t.Fatalf("from = %v, %v", from, err)
	}
	to, err := ParseRange("2026-11-06", true)
	if err != nil || to.Hour() != 23 || to.Minute() != 59 {
		t.Fatalf("to = %v, %v", to, err)
	}
	if _, err := ParseRange("bad", false); err == nil {
		t.Fatal("expected error")
	}
}
