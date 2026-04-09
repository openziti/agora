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
	ID             string           `db:"id"`
	OrganizationID string           `db:"organization_id"`
	AccountID      string           `db:"account_id"`
	Description    *string          `db:"description"`
	Host           *string          `db:"host"`
	ZitiIdentityID string           `db:"ziti_identity_id"`
	State          EnvironmentState `db:"state"`
	LastSeenAt     *time.Time       `db:"last_seen_at"`
	CreatedAt      time.Time        `db:"created_at"`
	UpdatedAt      time.Time        `db:"updated_at"`
}

type TunnelState string

const (
	TunnelStateActive   TunnelState = "active"
	TunnelStateDisabled TunnelState = "disabled"
)

type Tunnel struct {
	ID             string      `db:"id"`
	OrganizationID string      `db:"organization_id"`
	EnvironmentID  string      `db:"environment_id"`
	Name           string      `db:"name"`
	BackendAddress string      `db:"backend_address"`
	ZitiServiceID  *string     `db:"ziti_service_id"`
	State          TunnelState `db:"state"`
	CreatedAt      time.Time   `db:"created_at"`
	UpdatedAt      time.Time   `db:"updated_at"`
}
