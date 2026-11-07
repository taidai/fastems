package appapi

import (
	"strings"
	"testing"
)

func TestResolveAssetCode(t *testing.T) {
	t.Run("auto generate when empty", func(t *testing.T) {
		code, err := resolveAssetCode("", "")
		if err != nil {
			t.Fatal(err)
		}
		if code == "" {
			t.Fatal("auto asset_code must not be empty")
		}
	})

	t.Run("first manual override", func(t *testing.T) {
		code, err := resolveAssetCode("", "ES01-CAB01")
		if err != nil || code != "ES01-CAB01" {
			t.Fatalf("manual override failed: %q %v", code, err)
		}
	})

	t.Run("keep existing when no request", func(t *testing.T) {
		code, err := resolveAssetCode("ES01-CAB01", "")
		if err != nil || code != "ES01-CAB01" {
			t.Fatalf("keep existing failed: %q %v", code, err)
		}
	})

	t.Run("same value is idempotent", func(t *testing.T) {
		code, err := resolveAssetCode("ES01-CAB01", "ES01-CAB01")
		if err != nil || code != "ES01-CAB01" {
			t.Fatalf("idempotent failed: %q %v", code, err)
		}
	})

	t.Run("locked after first set", func(t *testing.T) {
		_, err := resolveAssetCode("ES01-CAB01", "ES01-CAB02")
		if err == nil || !strings.Contains(err.Error(), "已锁定") {
			t.Fatalf("expected lock error, got %v", err)
		}
	})
}

func TestCapabilityRoundTrip(t *testing.T) {
	original := Capability{
		Kind:       "discharge",
		Direction:  "discharge",
		PowerMinKW: 0,
		PowerMaxKW: 250,
		RampKWs:    10,
		Allowed:    true,
		LimitReason: "",
	}
	encoded, err := EncodeCapability(original)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeCapability(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Kind != original.Kind || decoded.PowerMaxKW != original.PowerMaxKW || !decoded.Allowed {
		t.Fatalf("round trip mismatch: %+v", decoded)
	}
}

func TestDecodeCapabilityEmptyIsZero(t *testing.T) {
	capability, err := DecodeCapability("")
	if err != nil {
		t.Fatal(err)
	}
	if capability.Allowed {
		t.Fatal("empty capability must decode to unavailable")
	}
}
