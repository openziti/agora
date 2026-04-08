package automation

import (
	"strconv"

	"github.com/google/uuid"
	"github.com/openziti/edge-api/rest_model"
)

type Tags struct {
	values map[string]interface{}
}

func NewTags() *Tags {
	return &Tags{values: map[string]interface{}{}}
}

func AgoraTags(version string) *Tags {
	if version == "" {
		version = DefaultAgoraVersion
	}
	return NewTags().
		WithTag("agora", true).
		WithTag("agoraVersion", version)
}

func (t *Tags) WithTag(key string, value interface{}) *Tags {
	t.values[key] = value
	return t
}

func (t *Tags) WithResourceKind(kind string) *Tags {
	return t.WithTag("agoraResourceKind", kind)
}

func (t *Tags) WithOrganizationID(id uuid.UUID) *Tags {
	return t.WithTag("agoraOrganizationId", id.String())
}

func (t *Tags) WithAccountID(id uuid.UUID) *Tags {
	return t.WithTag("agoraAccountId", id.String())
}

func (t *Tags) WithEnvironmentID(id uuid.UUID) *Tags {
	return t.WithTag("agoraEnvironmentId", id.String())
}

func (t *Tags) WithTunnelID(id uuid.UUID) *Tags {
	return t.WithTag("agoraTunnelId", id.String())
}

func (t *Tags) WithTunnelName(name string) *Tags {
	return t.WithTag("agoraTunnelName", name)
}

func (t *Tags) ToRESTModel() *rest_model.Tags {
	return &rest_model.Tags{SubTags: t.values}
}

func BuildFilter(field, value string) string {
	return field + "=\"" + value + "\""
}

func BuildTagFilter(tag, value string) string {
	return "tags." + tag + "=\"" + value + "\""
}

func BuildBoolTagFilter(tag string, value bool) string {
	return "tags." + tag + "=" + strconv.FormatBool(value)
}
