package framelog

import "testing"

func TestRingBufferNewestFirst(t *testing.T) {
	log := New(2)
	log.Record("peer", "tx", []byte{1})
	log.Record("peer", "rx", []byte{2})
	log.Record("peer", "tx", []byte{3})
	frames := log.List(10)
	if len(frames) != 2 {
		t.Fatalf("frames = %d", len(frames))
	}
	if frames[0].Hex != "03" || frames[1].Hex != "02" {
		t.Fatalf("frames = %+v", frames)
	}
	log.Clear()
	if len(log.List(10)) != 0 {
		t.Fatal("clear failed")
	}
}
