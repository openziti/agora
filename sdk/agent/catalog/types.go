package catalog

import "time"

// Capability describes one capability the advertised endpoint exposes.
type Capability struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// InteractionPatternKind identifies how consumers interact with an endpoint.
type InteractionPatternKind string

const (
	// InteractionRequestResponse is a request/response interaction pattern.
	InteractionRequestResponse InteractionPatternKind = "request-response"

	// InteractionStream is a streaming interaction pattern.
	InteractionStream InteractionPatternKind = "stream"

	// InteractionBroadcast is a broadcast interaction pattern.
	InteractionBroadcast InteractionPatternKind = "broadcast"

	// InteractionCustom is a caller-defined interaction pattern.
	InteractionCustom InteractionPatternKind = "custom"
)

// InteractionPattern describes how a consumer interacts with the advertised endpoint.
type InteractionPattern struct {
	Kind          InteractionPatternKind `json:"kind"`
	CustomPattern string                 `json:"customPattern,omitempty"`
}

// TunnelMode is the transport mode the advertised endpoint speaks.
type TunnelMode string

const (
	// TunnelTCP is a TCP tunnel mode advertisement.
	TunnelTCP TunnelMode = "tcp"

	// TunnelHTTP is an HTTP tunnel mode advertisement.
	TunnelHTTP TunnelMode = "http"

	// TunnelUDP is a UDP tunnel mode advertisement.
	TunnelUDP TunnelMode = "udp"
)

// AdvertisementStatus is the lifecycle state of a published advertisement.
type AdvertisementStatus string

const (
	// StatusActive indicates an advertisement is visible and publishable.
	StatusActive AdvertisementStatus = "active"

	// StatusRetracted indicates an advertisement has been retracted.
	StatusRetracted AdvertisementStatus = "retracted"
)

// PublishSpec describes the advertisement an agent wants to publish.
type PublishSpec struct {
	Name                string               `json:"name"`
	Description         string               `json:"description,omitempty"`
	Capabilities        []Capability         `json:"capabilities"`
	InteractionPatterns []InteractionPattern `json:"interactionPatterns,omitempty"`
	WorkgroupScopeIDs   []string             `json:"workgroupScopeIds"`
	TunnelMode          TunnelMode           `json:"tunnelMode,omitempty"`
	ContractID          string               `json:"contractId,omitempty"`
}

// Advertisement is the public SDK view of an advertisement record.
type Advertisement struct {
	ID                  string               `json:"id"`
	OrganizationID      string               `json:"organizationId"`
	OrganizationName    string               `json:"organizationName"`
	AccountID           string               `json:"accountId"`
	Name                string               `json:"name"`
	Description         string               `json:"description,omitempty"`
	Capabilities        []Capability         `json:"capabilities"`
	InteractionPatterns []InteractionPattern `json:"interactionPatterns"`
	WorkgroupScopeIDs   []string             `json:"workgroupScopeIds"`
	TunnelMode          TunnelMode           `json:"tunnelMode,omitempty"`
	ContractID          string               `json:"contractId,omitempty"`
	SchemaVersion       int                  `json:"schemaVersion"`
	Status              AdvertisementStatus  `json:"status"`
	RetractedAt         time.Time            `json:"retractedAt,omitempty"`
	CreatedAt           time.Time            `json:"createdAt"`
	UpdatedAt           time.Time            `json:"updatedAt"`
}
