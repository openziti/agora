package session

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

// FrameVersion is the wire format version byte carried at the head of
// every envelope frame.
const FrameVersion byte = 0x01

// PlatformMaxEnvelopeBytes is the hard infrastructure ceiling on any
// single envelope's total frame size (header + payload + prefix). A
// contract's `max_envelope_bytes` may bound more strictly; the platform
// ceiling always applies.
const PlatformMaxEnvelopeBytes = 10 * 1024 * 1024 // 10 MiB

const framePrefixLen = 1 + 4 + 4

// Envelope is the structured message exchanged over a session's tunnel.
// The header fields are infrastructure-visible and contract-enforceable;
// the payload is opaque bytes.
type Envelope struct {
	SchemaVersion        int       `json:"schema_version"`
	EnvelopeID           string    `json:"envelope_id"`
	SessionID            string    `json:"session_id"`
	SenderAccountID      string    `json:"sender_account_id"`
	SenderOrganizationID string    `json:"sender_organization_id"`
	MessageType          string    `json:"message_type"`
	ContentType          string    `json:"content_type,omitempty"`
	CorrelationID        string    `json:"correlation_id,omitempty"`
	Timestamp            time.Time `json:"timestamp"`
	Payload              []byte    `json:"-"`
}

// ErrUnknownFrameVersion is returned when a frame byte does not match a
// version this SDK can parse.
var ErrUnknownFrameVersion = errors.New("unknown frame version")

// ErrFrameTooLarge is returned when a frame's declared or serialized
// size exceeds the platform hard ceiling.
var ErrFrameTooLarge = errors.New("frame exceeds platform ceiling")

// ErrMalformedHeader is returned when the header JSON cannot be parsed.
var ErrMalformedHeader = errors.New("malformed envelope header")

// EncodeFrame writes one envelope frame to w. Returns the total byte
// count written (useful for contract-size enforcement).
func EncodeFrame(w io.Writer, env Envelope) (int, error) {
	headerJSON, err := json.Marshal(struct {
		SchemaVersion        int       `json:"schema_version"`
		EnvelopeID           string    `json:"envelope_id"`
		SessionID            string    `json:"session_id"`
		SenderAccountID      string    `json:"sender_account_id"`
		SenderOrganizationID string    `json:"sender_organization_id"`
		MessageType          string    `json:"message_type"`
		ContentType          string    `json:"content_type,omitempty"`
		CorrelationID        string    `json:"correlation_id,omitempty"`
		Timestamp            time.Time `json:"timestamp"`
	}{
		SchemaVersion:        env.SchemaVersion,
		EnvelopeID:           env.EnvelopeID,
		SessionID:            env.SessionID,
		SenderAccountID:      env.SenderAccountID,
		SenderOrganizationID: env.SenderOrganizationID,
		MessageType:          env.MessageType,
		ContentType:          env.ContentType,
		CorrelationID:        env.CorrelationID,
		Timestamp:            env.Timestamp,
	})
	if err != nil {
		return 0, fmt.Errorf("marshal envelope header: %w", err)
	}

	total := framePrefixLen + len(headerJSON) + len(env.Payload)
	if total > PlatformMaxEnvelopeBytes {
		return 0, fmt.Errorf("%w: %d > %d", ErrFrameTooLarge, total, PlatformMaxEnvelopeBytes)
	}

	var prefix [framePrefixLen]byte
	prefix[0] = FrameVersion
	binary.BigEndian.PutUint32(prefix[1:5], uint32(len(headerJSON)))
	binary.BigEndian.PutUint32(prefix[5:9], uint32(len(env.Payload)))

	if _, err := w.Write(prefix[:]); err != nil {
		return 0, err
	}
	if _, err := w.Write(headerJSON); err != nil {
		return 0, err
	}
	if len(env.Payload) > 0 {
		if _, err := w.Write(env.Payload); err != nil {
			return 0, err
		}
	}
	return total, nil
}

// DecodeFrame reads one envelope frame from r. Returns the parsed
// envelope and the total byte count consumed (useful for enforcement).
// Returns io.EOF when the stream closes cleanly between frames.
func DecodeFrame(r io.Reader) (Envelope, int, error) {
	var prefix [framePrefixLen]byte
	if _, err := io.ReadFull(r, prefix[:]); err != nil {
		return Envelope{}, 0, err
	}
	if prefix[0] != FrameVersion {
		return Envelope{}, framePrefixLen, fmt.Errorf("%w: 0x%02x", ErrUnknownFrameVersion, prefix[0])
	}
	headerLen := binary.BigEndian.Uint32(prefix[1:5])
	payloadLen := binary.BigEndian.Uint32(prefix[5:9])
	total := framePrefixLen + int(headerLen) + int(payloadLen)
	if total > PlatformMaxEnvelopeBytes {
		return Envelope{}, framePrefixLen, fmt.Errorf("%w: declared %d > %d", ErrFrameTooLarge, total, PlatformMaxEnvelopeBytes)
	}

	headerBytes := make([]byte, headerLen)
	if _, err := io.ReadFull(r, headerBytes); err != nil {
		return Envelope{}, framePrefixLen, fmt.Errorf("read header: %w", err)
	}
	var env Envelope
	if err := json.Unmarshal(headerBytes, &env); err != nil {
		return Envelope{}, framePrefixLen + int(headerLen), fmt.Errorf("%w: %v", ErrMalformedHeader, err)
	}

	if payloadLen > 0 {
		payload := make([]byte, payloadLen)
		if _, err := io.ReadFull(r, payload); err != nil {
			return env, framePrefixLen + int(headerLen), fmt.Errorf("read payload: %w", err)
		}
		env.Payload = payload
	}
	return env, total, nil
}
