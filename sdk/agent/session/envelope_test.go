package session

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"time"
)

func TestEnvelopeFrameRoundTrip(t *testing.T) {
	t.Parallel()
	env := Envelope{
		SchemaVersion:        1,
		EnvelopeID:           "evp_aaaaaaaaaaaa",
		SessionID:            "ses_bbbbbbbbbbbb",
		SenderAccountID:      "ac_cccccccccccc",
		SenderOrganizationID: "org_dddddddddddd",
		MessageType:          "markets.equity.request",
		ContentType:          "application/json",
		CorrelationID:        "req-1",
		Timestamp:            time.Now().UTC().Truncate(time.Second),
		Payload:              []byte(`{"symbol":"SPY"}`),
	}
	var buf bytes.Buffer
	n, err := EncodeFrame(&buf, env)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if n != buf.Len() {
		t.Fatalf("encoded length mismatch: %d vs %d", n, buf.Len())
	}

	decoded, m, err := DecodeFrame(&buf)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if m != n {
		t.Fatalf("decoded byte count mismatch: %d vs %d", m, n)
	}
	if decoded.MessageType != env.MessageType {
		t.Fatalf("message_type round-trip failed: %q", decoded.MessageType)
	}
	if decoded.EnvelopeID != env.EnvelopeID {
		t.Fatalf("envelope_id round-trip failed: %q", decoded.EnvelopeID)
	}
	if string(decoded.Payload) != string(env.Payload) {
		t.Fatalf("payload round-trip failed: %q", decoded.Payload)
	}
	if !decoded.Timestamp.Equal(env.Timestamp) {
		t.Fatalf("timestamp round-trip failed: %v vs %v", decoded.Timestamp, env.Timestamp)
	}
}

func TestEnvelopeFrameEmptyPayload(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if _, err := EncodeFrame(&buf, Envelope{SchemaVersion: 1, MessageType: "ping"}); err != nil {
		t.Fatalf("encode: %v", err)
	}
	dec, _, err := DecodeFrame(&buf)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(dec.Payload) != 0 {
		t.Fatalf("expected empty payload, got %d bytes", len(dec.Payload))
	}
}

func TestEnvelopeFrameUnknownVersion(t *testing.T) {
	t.Parallel()
	buf := bytes.NewReader([]byte{0x02, 0, 0, 0, 0, 0, 0, 0, 0})
	_, _, err := DecodeFrame(buf)
	if !errors.Is(err, ErrUnknownFrameVersion) {
		t.Fatalf("expected ErrUnknownFrameVersion, got %v", err)
	}
}

func TestEnvelopeFrameTooLarge(t *testing.T) {
	t.Parallel()
	// encode too-large: claim 200 MB of payload
	b := []byte{0x01, 0, 0, 0, 0, 0x0c, 0x00, 0x00, 0x00}
	_, _, err := DecodeFrame(bytes.NewReader(b))
	if !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("expected ErrFrameTooLarge, got %v", err)
	}
}

func TestEnvelopeFrameMalformedHeader(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	buf.Write([]byte{0x01, 0, 0, 0, 5, 0, 0, 0, 0})
	buf.WriteString("not{}")
	_, _, err := DecodeFrame(&buf)
	if !errors.Is(err, ErrMalformedHeader) {
		t.Fatalf("expected ErrMalformedHeader, got %v", err)
	}
}

func TestEnvelopeFrameEOFBetweenFrames(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	_, _, err := DecodeFrame(&buf)
	if err != io.EOF {
		t.Fatalf("expected io.EOF at stream end, got %v", err)
	}
}