package tunnel

import "time"

// Mode is the transport mode a tunnel speaks.
type Mode string

const (
	// ModeTCP is a raw TCP tunnel.
	ModeTCP Mode = "tcp"

	// ModeHTTP is an HTTP reverse-proxy tunnel.
	ModeHTTP Mode = "http"

	// ModeUDP is a UDP tunnel.
	ModeUDP Mode = "udp"
)

// State is the lifecycle state of a serve or connect actor.
type State string

const (
	// StateConfigured means the actor is configured but not running.
	StateConfigured State = "configured"

	// StateStarting means the runtime is attempting to start the actor.
	StateStarting State = "starting"

	// StateRunning means the actor is currently running.
	StateRunning State = "running"

	// StateError means the actor failed and may retry in the background.
	StateError State = "error"

	// StateStopped means the actor has stopped.
	StateStopped State = "stopped"
)

// ServeSpec describes a tunnel serve the caller wants to ensure exists.
type ServeSpec struct {
	// Name is the service name as advertised to the controller.
	Name string

	// Mode is the transport mode. Required.
	Mode Mode

	// BackendTarget is the local target the runtime forwards to.
	BackendTarget string

	// GrantEmails is the optional additive list of account emails
	// granted access to the served tunnel.
	GrantEmails []string
}

// ServeStatus reflects the runtime's view of a serve actor.
type ServeStatus struct {
	Name          string
	Mode          Mode
	BackendTarget string
	TunnelID      string
	ServeID       string
	State         State
	LastError     string
	LastStartedAt time.Time
	NextRetryAt   time.Time
	RetryAttempt  uint32
}

// ConnectSpec describes a tunnel connect the caller wants to ensure exists.
type ConnectSpec struct {
	// Name is the service name to dial.
	Name string

	// ListenAddress is the concrete local host:port the runtime binds.
	ListenAddress string
}

// ConnectStatus reflects the runtime's view of a connect actor.
type ConnectStatus struct {
	Name          string
	ListenAddress string
	TunnelID      string
	AttachmentID  string
	State         State
	LastError     string
	LastStartedAt time.Time
	NextRetryAt   time.Time
	RetryAttempt  uint32
}
