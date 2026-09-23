package plugin

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDecodeUsageUpstreamTiming(t *testing.T) {
	raw := []byte(`{"RequestedAt":"2026-09-24T00:00:00Z","Latency":"2s","TTFT":"1s","UpstreamTTFB":900000000,"FirstPacket":950000000,"ConnSetup":"120ms","ConnReused":true,"Detail":{"InputTokens":1,"OutputTokens":2}}`)
	decoded, err := decodeUsage(raw, time.Now())
	if err != nil {
		t.Fatalf("decodeUsage: %v", err)
	}
	if decoded.UpstreamTTFBNS != uint64(900*time.Millisecond) {
		t.Fatalf("upstream ttfb = %d, want %d", decoded.UpstreamTTFBNS, uint64(900*time.Millisecond))
	}
	if decoded.FirstPacketNS != uint64(950*time.Millisecond) {
		t.Fatalf("first packet = %d, want %d", decoded.FirstPacketNS, uint64(950*time.Millisecond))
	}
	if decoded.ConnSetupNS != uint64(120*time.Millisecond) {
		t.Fatalf("conn setup = %d, want %d", decoded.ConnSetupNS, uint64(120*time.Millisecond))
	}
	if !decoded.ConnReused {
		t.Fatal("conn reused = false, want true")
	}
}

func TestDecodeUsageUpstreamTimingMissing(t *testing.T) {
	raw := []byte(`{"Latency":"2s","TTFT":"1s"}`)
	decoded, err := decodeUsage(raw, time.Now())
	if err != nil {
		t.Fatalf("decodeUsage: %v", err)
	}
	if decoded.UpstreamTTFBNS != 0 || decoded.FirstPacketNS != 0 || decoded.ConnSetupNS != 0 {
		t.Fatalf("missing fields must decode to zero, got ttfb=%d first=%d setup=%d",
			decoded.UpstreamTTFBNS, decoded.FirstPacketNS, decoded.ConnSetupNS)
	}
	if decoded.ConnReused {
		t.Fatal("conn reused = true, want false when the field is missing")
	}
}

func TestRequestDetailForUsagePropagatesUpstreamTiming(t *testing.T) {
	raw := []byte(`{"Latency":"2s","TTFT":"1s","UpstreamTTFB":"800ms","FirstPacket":"820ms","ConnSetup":"50ms","ConnReused":false}`)
	decoded, err := decodeUsage(raw, time.Now())
	if err != nil {
		t.Fatalf("decodeUsage: %v", err)
	}
	item := requestDetailForUsage(decoded, 7)
	if item.UpstreamTTFBNS != uint64(800*time.Millisecond) {
		t.Fatalf("upstream ttfb = %d, want %d", item.UpstreamTTFBNS, uint64(800*time.Millisecond))
	}
	if item.FirstPacketNS != uint64(820*time.Millisecond) {
		t.Fatalf("first packet = %d, want %d", item.FirstPacketNS, uint64(820*time.Millisecond))
	}
	if item.ConnSetupNS != uint64(50*time.Millisecond) {
		t.Fatalf("conn setup = %d, want %d", item.ConnSetupNS, uint64(50*time.Millisecond))
	}
	if item.ConnReused {
		t.Fatal("conn reused = true, want false")
	}
}

func TestRequestDetailJSONRoundTripKeepsUpstreamTiming(t *testing.T) {
	item := RequestDetail{
		Sequence:       1,
		Time:           time.Unix(1700000000, 0).UTC(),
		LatencyNS:      uint64(2 * time.Second),
		TTFTNS:         uint64(time.Second),
		UpstreamTTFBNS: uint64(900 * time.Millisecond),
		FirstPacketNS:  uint64(950 * time.Millisecond),
		ConnSetupNS:    uint64(30 * time.Millisecond),
		ConnReused:     true,
	}
	encoded, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded RequestDetail
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.UpstreamTTFBNS != item.UpstreamTTFBNS || decoded.FirstPacketNS != item.FirstPacketNS ||
		decoded.ConnSetupNS != item.ConnSetupNS || decoded.ConnReused != item.ConnReused {
		t.Fatalf("round trip mismatch: got ttfb=%d first=%d setup=%d reused=%v",
			decoded.UpstreamTTFBNS, decoded.FirstPacketNS, decoded.ConnSetupNS, decoded.ConnReused)
	}
}
