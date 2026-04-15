package automation

import (
	"strconv"

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

func (t *Tags) WithOrganizationID(id string) *Tags {
	return t.WithTag("agoraOrganizationId", id)
}

func (t *Tags) WithAccountID(id string) *Tags {
	return t.WithTag("agoraAccountId", id)
}

func (t *Tags) WithEnvironmentID(id string) *Tags {
	return t.WithTag("agoraEnvironmentId", id)
}

func (t *Tags) WithTunnelID(id string) *Tags {
	return t.WithTag("agoraTunnelId", id)
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

func BuildTagExistsFilter(tag string) string {
	return "tags." + tag + " != null"
}

func BuildBoolTagFilter(tag string, value bool) string {
	return "tags." + tag + "=" + strconv.FormatBool(value)
}
