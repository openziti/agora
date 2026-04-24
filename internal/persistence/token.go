package persistence

import "crypto/rand"

// ResourcePrefix distinguishes the type of resource an identifier belongs to.
type ResourcePrefix string

const (
	PrefixOrganization        ResourcePrefix = "org_"
	PrefixAccount             ResourcePrefix = "ac_"
	PrefixEnvironment         ResourcePrefix = "ev_"
	PrefixTunnel              ResourcePrefix = "tt_"
	PrefixAttachment          ResourcePrefix = "ta_"
	PrefixTunnelServe         ResourcePrefix = "ts_"
	PrefixWorkgroup           ResourcePrefix = "wg_"
	PrefixWorkgroupInvitation ResourcePrefix = "wgi_"
	PrefixWorkgroupMembership ResourcePrefix = "wgm_"
	PrefixAdvertisement       ResourcePrefix = "adv_"
	PrefixSession             ResourcePrefix = "ses_"
	PrefixContract            ResourcePrefix = "con_"
)

const (
	tokenAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	tokenLength   = 12
)

// NewResourceID generates a prefixed resource identifier with 12 random
// lowercase alphanumeric characters.
func NewResourceID(prefix ResourcePrefix) string {
	buf := make([]byte, tokenLength)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	for i := range buf {
		buf[i] = tokenAlphabet[buf[i]%byte(len(tokenAlphabet))]
	}
	return string(prefix) + string(buf)
}
