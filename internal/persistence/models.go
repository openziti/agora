package persistence

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lib/pq"
)

type Organization struct {
	ID        string    `db:"id"`
	Name      string    `db:"name"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

type AuditEventType string

const (
	AuditEventSessionProposed        AuditEventType = "session.proposed"
	AuditEventSessionAccepted        AuditEventType = "session.accepted"
	AuditEventSessionRejected        AuditEventType = "session.rejected"
	AuditEventSessionClosed          AuditEventType = "session.closed"
	AuditEventEnvelopeFlowed         AuditEventType = "envelope.flowed"
	AuditEventTunnelAttached         AuditEventType = "tunnel.attached"
	AuditEventTunnelDetached         AuditEventType = "tunnel.detached"
	AuditEventAdvertisementPublished AuditEventType = "advertisement.published"
	AuditEventAdvertisementRetracted AuditEventType = "advertisement.retracted"
	AuditEventEnvironmentHeartbeat   AuditEventType = "environment.heartbeat"
	AuditEventAccountLogin           AuditEventType = "account.login"
	AuditEventAccountLoginFailed     AuditEventType = "account.login_failed"
	AuditEventAccountLogout          AuditEventType = "account.logout"
)

type AuditEventData map[string]any

func (d *AuditEventData) Scan(src any) error { return scanJSONB(src, d) }
func (d AuditEventData) Value() (driver.Value, error) {
	if d == nil {
		return []byte("{}"), nil
	}
	return marshalJSONB(d)
}

type AuditEvent struct {
	ID              int64          `db:"id"`
	OccurredAt      time.Time      `db:"occurred_at"`
	EventType       AuditEventType `db:"event_type"`
	OrganizationID  string         `db:"organization_id"`
	AccountID       *string        `db:"account_id"`
	WorkgroupID     *string        `db:"workgroup_id"`
	SessionID       *string        `db:"session_id"`
	AdvertisementID *string        `db:"advertisement_id"`
	ContractID      *string        `db:"contract_id"`
	EnvelopeID      *string        `db:"envelope_id"`
	Data            AuditEventData `db:"data"`
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

type TunnelKind string

const (
	TunnelKindProxy  TunnelKind = "proxy"
	TunnelKindDirect TunnelKind = "direct"
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
	Kind                      TunnelKind  `db:"kind"`
	BackendTarget             *string     `db:"backend_target"`
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

type TunnelServeState string

const (
	TunnelServeStateActive       TunnelServeState = "active"
	TunnelServeStateStale        TunnelServeState = "stale"
	TunnelServeStateDisconnected TunnelServeState = "disconnected"
)

type TunnelServe struct {
	ID              string           `db:"id"`
	TunnelID        string           `db:"tunnel_id"`
	OrganizationID  string           `db:"organization_id"`
	AccountID       string           `db:"account_id"`
	EnvironmentID   string           `db:"environment_id"`
	State           TunnelServeState `db:"state"`
	LastHeartbeatAt time.Time        `db:"last_heartbeat_at"`
	DisconnectedAt  *time.Time       `db:"disconnected_at"`
	Deleted         bool             `db:"deleted"`
	CreatedAt       time.Time        `db:"created_at"`
	UpdatedAt       time.Time        `db:"updated_at"`
}

type TunnelServeDetail struct {
	TunnelServe
	EnvironmentHost *string `db:"environment_host"`
	TunnelName      string  `db:"tunnel_name"`
	TunnelMode      string  `db:"tunnel_mode"`
}

type WorkgroupScope string

const (
	WorkgroupScopeIntraOrg WorkgroupScope = "intra-org"
	WorkgroupScopeInterOrg WorkgroupScope = "inter-org"
)

type WorkgroupState string

const (
	WorkgroupStatePending  WorkgroupState = "pending"
	WorkgroupStateActive   WorkgroupState = "active"
	WorkgroupStateDeclined WorkgroupState = "declined"
)

type Workgroup struct {
	ID                  string         `db:"id"`
	OwnerOrganizationID string         `db:"owner_organization_id"`
	Name                string         `db:"name"`
	Description         *string        `db:"description"`
	Scope               WorkgroupScope `db:"scope"`
	State               WorkgroupState `db:"state"`
	Deleted             bool           `db:"deleted"`
	CreatedAt           time.Time      `db:"created_at"`
	UpdatedAt           time.Time      `db:"updated_at"`
}

type WorkgroupInvitationState string

const (
	WorkgroupInvitationStatePending  WorkgroupInvitationState = "pending"
	WorkgroupInvitationStateAccepted WorkgroupInvitationState = "accepted"
	WorkgroupInvitationStateDeclined WorkgroupInvitationState = "declined"
)

type WorkgroupInvitation struct {
	ID                      string                   `db:"id"`
	WorkgroupID             string                   `db:"workgroup_id"`
	OrganizationID          string                   `db:"organization_id"`
	State                   WorkgroupInvitationState `db:"state"`
	AcknowledgedByAccountID *string                  `db:"acknowledged_by_account_id"`
	AcknowledgedAt          *time.Time               `db:"acknowledged_at"`
	CreatedAt               time.Time                `db:"created_at"`
	UpdatedAt               time.Time                `db:"updated_at"`
}

type WorkgroupMembershipRole string

const (
	WorkgroupMembershipRoleMember WorkgroupMembershipRole = "member"
	WorkgroupMembershipRoleAdmin  WorkgroupMembershipRole = "admin"
)

type WorkgroupMembership struct {
	ID             string                  `db:"id"`
	WorkgroupID    string                  `db:"workgroup_id"`
	OrganizationID string                  `db:"organization_id"`
	AccountID      string                  `db:"account_id"`
	Role           WorkgroupMembershipRole `db:"role"`
	JoinedAt       time.Time               `db:"joined_at"`
	Deleted        bool                    `db:"deleted"`
	CreatedAt      time.Time               `db:"created_at"`
	UpdatedAt      time.Time               `db:"updated_at"`
}

type AdvertisementStatus string

const (
	AdvertisementStatusActive    AdvertisementStatus = "active"
	AdvertisementStatusRetracted AdvertisementStatus = "retracted"
)

type InteractionPatternKind string

const (
	InteractionPatternKindRequestResponse InteractionPatternKind = "request-response"
	InteractionPatternKindStream          InteractionPatternKind = "stream"
	InteractionPatternKindBroadcast       InteractionPatternKind = "broadcast"
	InteractionPatternKindCustom          InteractionPatternKind = "custom"
)

type Capability struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type InteractionPattern struct {
	Kind          InteractionPatternKind `json:"kind"`
	CustomPattern string                 `json:"customPattern,omitempty"`
}

// CapabilitiesJSON wraps []Capability so it can be scanned from and
// stored to a jsonb column via database/sql.
type CapabilitiesJSON []Capability

func (c *CapabilitiesJSON) Scan(src any) error { return scanJSONB(src, c) }
func (c CapabilitiesJSON) Value() (driver.Value, error) {
	return marshalJSONB(c)
}

// InteractionPatternsJSON wraps []InteractionPattern for jsonb storage.
type InteractionPatternsJSON []InteractionPattern

func (p *InteractionPatternsJSON) Scan(src any) error { return scanJSONB(src, p) }
func (p InteractionPatternsJSON) Value() (driver.Value, error) {
	return marshalJSONB(p)
}

func scanJSONB(src any, dst any) error {
	if src == nil {
		return nil
	}
	var data []byte
	switch v := src.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return fmt.Errorf("unsupported scan type %T for jsonb", src)
	}
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, dst)
}

func marshalJSONB(src any) (driver.Value, error) {
	bytes, err := json.Marshal(src)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}

type SessionState string

const (
	SessionStateProposed  SessionState = "proposed"
	SessionStateAccepting SessionState = "accepting"
	SessionStateActive    SessionState = "active"
	SessionStateClosing   SessionState = "closing"
	SessionStateClosed    SessionState = "closed"
)

type SessionCloseReason string

const (
	SessionCloseReasonRejected            SessionCloseReason = "rejected"
	SessionCloseReasonConsumerClose       SessionCloseReason = "consumer_close"
	SessionCloseReasonProviderClose       SessionCloseReason = "provider_close"
	SessionCloseReasonContractViolation   SessionCloseReason = "contract_violation"
	SessionCloseReasonTunnelFailed        SessionCloseReason = "tunnel_failed"
	SessionCloseReasonAdminClose          SessionCloseReason = "admin_close"
	SessionCloseReasonWorkgroupDeleted    SessionCloseReason = "workgroup_deleted"
	SessionCloseReasonEnvironmentDisabled SessionCloseReason = "environment_disabled"
)

type Session struct {
	ID                       string              `db:"id"`
	AdvertisementID          string              `db:"advertisement_id"`
	WorkgroupID              string              `db:"workgroup_id"`
	ProviderAccountID        string              `db:"provider_account_id"`
	ProviderOrganizationID   string              `db:"provider_organization_id"`
	ConsumerAccountID        string              `db:"consumer_account_id"`
	ConsumerOrganizationID   string              `db:"consumer_organization_id"`
	AdvertisementName        string              `db:"advertisement_name"`
	WorkgroupName            string              `db:"workgroup_name"`
	ProviderOrganizationName string              `db:"provider_organization_name"`
	ConsumerOrganizationName string              `db:"consumer_organization_name"`
	ProviderAccountEmail     *string             `db:"provider_account_email"`
	ConsumerAccountEmail     *string             `db:"consumer_account_email"`
	TunnelMode               TunnelMode          `db:"tunnel_mode"`
	TunnelID                 *string             `db:"tunnel_id"`
	ContractSnapshotJSON     []byte              `db:"contract_snapshot"`
	EnvelopeCount            *int                `db:"envelope_count"`
	State                    SessionState        `db:"state"`
	CloseReason              *SessionCloseReason `db:"close_reason"`
	CloseDetail              *string             `db:"close_detail"`
	ProposerMessage          *string             `db:"proposer_message"`
	ProposedAt               time.Time           `db:"proposed_at"`
	AcceptedAt               *time.Time          `db:"accepted_at"`
	ClosedAt                 *time.Time          `db:"closed_at"`
}

type Advertisement struct {
	ID                  string                  `db:"id"`
	OrganizationID      string                  `db:"organization_id"`
	OrganizationName    string                  `db:"organization_name"`
	AccountID           string                  `db:"account_id"`
	Name                string                  `db:"name"`
	Description         *string                 `db:"description"`
	Capabilities        CapabilitiesJSON        `db:"capabilities"`
	InteractionPatterns InteractionPatternsJSON `db:"interaction_patterns"`
	WorkgroupScopes     pq.StringArray          `db:"workgroup_scopes"`
	TunnelMode          TunnelMode              `db:"tunnel_mode"`
	ContractID          *string                 `db:"contract_id"`
	SchemaVersion       int                     `db:"schema_version"`
	Status              AdvertisementStatus     `db:"status"`
	RetractedAt         *time.Time              `db:"retracted_at"`
	CreatedAt           time.Time               `db:"created_at"`
	UpdatedAt           time.Time               `db:"updated_at"`
}

type ContractAccessMode string

const (
	ContractAccessModeOpen             ContractAccessMode = "open"
	ContractAccessModeApprovalRequired ContractAccessMode = "approval_required"
)

// MaturityRequirements is the structured object carried in the jsonb
// maturity_requirements column. Zero-value MinAccountAgeDays means no
// gate; the field is omitempty in the JSON representation so the
// "absent" case round-trips cleanly.
type MaturityRequirements struct {
	MinAccountAgeDays int `json:"minAccountAgeDays,omitempty"`
}

// MaturityRequirementsJSON wraps MaturityRequirements so it can be
// scanned from / stored to a jsonb column via database/sql.
type MaturityRequirementsJSON MaturityRequirements

func (m *MaturityRequirementsJSON) Scan(src any) error { return scanJSONB(src, m) }
func (m MaturityRequirementsJSON) Value() (driver.Value, error) {
	return marshalJSONB(m)
}

type Contract struct {
	ID                           string                   `db:"id"`
	AccountID                    string                   `db:"account_id"`
	OrganizationID               string                   `db:"organization_id"`
	Name                         string                   `db:"name"`
	Description                  *string                  `db:"description"`
	SchemaVersion                int                      `db:"schema_version"`
	MaxDurationSeconds           int                      `db:"max_duration_seconds"`
	MaxEnvelopeCount             int                      `db:"max_envelope_count"`
	MaxEnvelopeBytes             int                      `db:"max_envelope_bytes"`
	AllowedMessageTypes          pq.StringArray           `db:"allowed_message_types"`
	RequiredWorkgroupMemberships pq.StringArray           `db:"required_workgroup_memberships"`
	MaturityRequirements         MaturityRequirementsJSON `db:"maturity_requirements"`
	AccessMode                   ContractAccessMode       `db:"access_mode"`
	CreatedAt                    time.Time                `db:"created_at"`
	UpdatedAt                    time.Time                `db:"updated_at"`
}

// ContractSnapshotJSON is the frozen shape stored on sessions.contract_snapshot.
// It mirrors Contract minus the ownership and resource-timestamp fields.
type ContractSnapshot struct {
	ContractID                   string               `json:"contractId"`
	Name                         string               `json:"name"`
	Description                  string               `json:"description,omitempty"`
	SchemaVersion                int                  `json:"schemaVersion"`
	MaxDurationSeconds           int                  `json:"maxDurationSeconds"`
	MaxEnvelopeCount             int                  `json:"maxEnvelopeCount"`
	MaxEnvelopeBytes             int                  `json:"maxEnvelopeBytes"`
	AllowedMessageTypes          []string             `json:"allowedMessageTypes"`
	RequiredWorkgroupMemberships []string             `json:"requiredWorkgroupMemberships"`
	MaturityRequirements         MaturityRequirements `json:"maturityRequirements"`
	AccessMode                   ContractAccessMode   `json:"accessMode"`
	SnapshottedAt                time.Time            `json:"snapshottedAt"`
}
