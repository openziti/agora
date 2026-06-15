package controller

import (
	"context"

	"github.com/michaelquigley/push/build"
	"github.com/openziti/agora/internal/api"
)

// GetVersion implements the unauthenticated GET /v1/version operation. It reports the
// controller's build metadata from the push build package, which is stamped at release
// time (goreleaser) or CI build time (push ci/ldflags.sh) via ldflags and falls back to
// the developer-build identifier when unstamped. The optional fields are only populated
// when the corresponding values were stamped.
func (s *Service) GetVersion(_ context.Context) (api.GetVersionRes, error) {
	info := &api.VersionInfo{Version: build.String()}
	if build.Hash != "" {
		info.Hash = api.NewOptString(build.Hash)
	}
	if build.Date != "" {
		info.Date = api.NewOptString(build.Date)
	}
	if build.Builder != "" {
		info.Builder = api.NewOptString(build.Builder)
	}
	if build.Branch != "" {
		info.Branch = api.NewOptString(build.Branch)
	}
	return info, nil
}
