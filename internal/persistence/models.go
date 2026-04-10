package persistence

import "time"

type Organization struct {
	ID        string    `db:"id"`
	Name      string    `db:"name"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

type AccountRole string

const (
	AccountRoleAdmin  AccountRole = "admin"
	AccountRoleMember AccountRole = "member"
)

type AccountStatus string

const (
	AccountStatusActive   AccountStatus = "active"
	AccountStatusDisabled AccountStatus = "disabled"
)

type Account struct {
	ID             string        `db:"id"`
	OrganizationID string        `db:"organization_id"`
	Email          string        `db:"email"`
	DisplayName    *string       `db:"display_name"`
	PasswordSalt   string        `db:"password_salt"`
	PasswordHash   string        `db:"password_hash"`
	AccountToken   string        `db:"account_token"`
	Role           AccountRole   `db:"role"`
	Status         AccountStatus `db:"status"`
	CreatedAt      time.Time     `db:"created_at"`
	UpdatedAt      time.Time     `db:"updated_at"`
}

type EnvironmentState string

const (
	EnvironmentStateEnabled  EnvironmentState = "enabled"
	EnvironmentStateDisabled EnvironmentState = "disabled"
)

type Environment struct {
	ID                 string           `db:"id"`
	OrganizationID     string           `db:"organization_id"`
	AccountID          string           `db:"account_id"`
	Description        *string          `db:"description"`
	Host               *string          `db:"host"`
	ZitiIdentityID     string           `db:"ziti_identity_id"`
	EdgeRouterPolicyID *string          `db:"edge_router_policy_id"`
	State              EnvironmentState `db:"state"`
	Deleted            bool             `db:"deleted"`
	LastSeenAt         *time.Time       `db:"last_seen_at"`
	CreatedAt          time.Time        `db:"created_at"`
	UpdatedAt          time.Time        `db:"updated_at"`
}

type TunnelState string

const (
	TunnelStateActive   TunnelState = "active"
	TunnelStateDisabled TunnelState = "disabled"
)

type TunnelMode string

const (
	TunnelModeHTTP TunnelMode = "http"
	TunnelModeTCP  TunnelMode = "tcp"
	TunnelModeUDP  TunnelMode = "udp"
)

type Tunnel struct {
	ID                        string      `db:"id"`
	OrganizationID            string      `db:"organization_id"`
	AccountID                 string      `db:"account_id"`
	EnvironmentID             string      `db:"environment_id"`
	Name                      string      `db:"name"`
	Mode                      TunnelMode  `db:"mode"`
	BackendTarget             string      `db:"backend_target"`
	ZitiServiceID             *string     `db:"ziti_service_id"`
	BindPolicyID              *string     `db:"bind_policy_id"`
	ServiceEdgeRouterPolicyID *string     `db:"service_edge_router_policy_id"`
	State                     TunnelState `db:"state"`
	Deleted                   bool        `db:"deleted"`
	CreatedAt                 time.Time   `db:"created_at"`
	UpdatedAt                 time.Time   `db:"updated_at"`
}

type TunnelAccountGrant struct {
	TunnelID       string    `db:"tunnel_id"`
	AccountID      string    `db:"account_id"`
	OrganizationID string    `db:"organization_id"`
	Deleted        bool      `db:"deleted"`
	CreatedAt      time.Time `db:"created_at"`
	UpdatedAt      time.Time `db:"updated_at"`
}

type TunnelGrant struct {
	TunnelID  string    `db:"tunnel_id"`
	AccountID string    `db:"account_id"`
	Email     string    `db:"email"`
	CreatedAt time.Time `db:"created_at"`
}

type TunnelAttachmentState string

const (
	TunnelAttachmentStateActive       TunnelAttachmentState = "active"
	TunnelAttachmentStateStale        TunnelAttachmentState = "stale"
	TunnelAttachmentStateDisconnected TunnelAttachmentState = "disconnected"
)

type TunnelAttachment struct {
	ID              string                `db:"id"`
	TunnelID        string                `db:"tunnel_id"`
	OrganizationID  string                `db:"organization_id"`
	AccountID       string                `db:"account_id"`
	EnvironmentID   string                `db:"environment_id"`
	ListenAddress   string                `db:"listen_address"`
	DialPolicyID    *string               `db:"dial_policy_id"`
	State           TunnelAttachmentState `db:"state"`
	LastHeartbeatAt time.Time             `db:"last_heartbeat_at"`
	DisconnectedAt  *time.Time            `db:"disconnected_at"`
	Deleted         bool                  `db:"deleted"`
	CreatedAt       time.Time             `db:"created_at"`
	UpdatedAt       time.Time             `db:"updated_at"`
}

type TunnelAttachmentDetail struct {
	TunnelAttachment
	AccountEmail string     `db:"account_email"`
	TunnelName   string     `db:"tunnel_name"`
	TunnelMode   TunnelMode `db:"tunnel_mode"`
}
