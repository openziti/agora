package persistence

import (
	"time"

	"github.com/google/uuid"
)

type Organization struct {
	ID        uuid.UUID `db:"id"`
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
	ID             uuid.UUID     `db:"id"`
	OrganizationID uuid.UUID     `db:"organization_id"`
	Email          string        `db:"email"`
	DisplayName    *string       `db:"display_name"`
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
	ID             uuid.UUID        `db:"id"`
	OrganizationID uuid.UUID        `db:"organization_id"`
	AccountID      uuid.UUID        `db:"account_id"`
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
	ID             uuid.UUID   `db:"id"`
	OrganizationID uuid.UUID   `db:"organization_id"`
	EnvironmentID  uuid.UUID   `db:"environment_id"`
	Name           string      `db:"name"`
	BackendAddress string      `db:"backend_address"`
	ZitiServiceID  *string     `db:"ziti_service_id"`
	State          TunnelState `db:"state"`
	CreatedAt      time.Time   `db:"created_at"`
	UpdatedAt      time.Time   `db:"updated_at"`
}
