package catalog

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/openziti/agora/internal/api"
)

type fakeController struct {
	publish func(context.Context, *api.PublishAdvertisementRequest) (api.PublishAdvertisementRes, error)
	list    func(context.Context, api.ListAdvertisementsParams) (api.ListAdvertisementsRes, error)
	retract func(context.Context, api.RetractAdvertisementParams) (api.RetractAdvertisementRes, error)
}

func (f *fakeController) PublishAdvertisement(ctx context.Context, req *api.PublishAdvertisementRequest) (api.PublishAdvertisementRes, error) {
	if f.publish == nil {
		panic("unexpected PublishAdvertisement call")
	}
	return f.publish(ctx, req)
}

func (f *fakeController) ListAdvertisements(ctx context.Context, params api.ListAdvertisementsParams) (api.ListAdvertisementsRes, error) {
	if f.list == nil {
		panic("unexpected ListAdvertisements call")
	}
	return f.list(ctx, params)
}

func (f *fakeController) RetractAdvertisement(ctx context.Context, params api.RetractAdvertisementParams) (api.RetractAdvertisementRes, error) {
	if f.retract == nil {
		panic("unexpected RetractAdvertisement call")
	}
	return f.retract(ctx, params)
}

func TestEnsurePublishedCreatesAdvertisement(t *testing.T) {
	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	fake := &fakeController{
		publish: func(_ context.Context, req *api.PublishAdvertisementRequest) (api.PublishAdvertisementRes, error) {
			if req.Name != "llm-gateway" {
				t.Fatalf("unexpected name %q", req.Name)
			}
			if req.Description.Value != "LLM routing gateway" {
				t.Fatalf("unexpected description %#v", req.Description)
			}
			if got := req.WorkgroupScopes; len(got) != 1 || got[0] != "wg_abcdefghijkl" {
				t.Fatalf("unexpected workgroup scopes %#v", got)
			}
			if req.ContractId.Value != "con_abcdefghijkl" {
				t.Fatalf("unexpected contract id %#v", req.ContractId)
			}
			if req.TunnelMode.Value != api.AdvertisementTunnelModeTCP {
				t.Fatalf("unexpected tunnel mode %#v", req.TunnelMode)
			}
			if len(req.InteractionPatterns) != 1 || req.InteractionPatterns[0].Kind != api.AdvertisementInteractionPatternKindRequestResponse {
				t.Fatalf("expected default request-response pattern, got %#v", req.InteractionPatterns)
			}
			if len(req.Capabilities) != 1 || req.Capabilities[0].Name != "llm-routing" {
				t.Fatalf("unexpected capabilities %#v", req.Capabilities)
			}
			if req.Capabilities[0].Metadata.Value["provider"] != "openai" {
				t.Fatalf("unexpected metadata %#v", req.Capabilities[0].Metadata)
			}
			return apiAdvertisement("adv_abcdefghijkl", "llm-gateway", now), nil
		},
	}

	got, err := ensurePublished(context.Background(), fake, PublishSpec{
		Name:              "llm-gateway",
		Description:       "LLM routing gateway",
		Capabilities:      []Capability{{Name: "llm-routing", Metadata: map[string]string{"provider": "openai"}}},
		WorkgroupScopeIDs: []string{"wg_abcdefghijkl"},
		TunnelMode:        TunnelTCP,
		ContractID:        "con_abcdefghijkl",
	})
	if err != nil {
		t.Fatalf("ensure published: %v", err)
	}
	if got.ID != "adv_abcdefghijkl" || got.Name != "llm-gateway" || got.TunnelMode != TunnelTCP {
		t.Fatalf("unexpected advertisement %#v", got)
	}
	if got.ContractID != "con_abcdefghijkl" || got.CreatedAt != now {
		t.Fatalf("unexpected mapped fields %#v", got)
	}
}

func TestEnsurePublishedConflictReturnsExisting(t *testing.T) {
	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	var listed bool
	fake := &fakeController{
		publish: func(context.Context, *api.PublishAdvertisementRequest) (api.PublishAdvertisementRes, error) {
			return &api.PublishAdvertisementConflict{Code: "name_in_use", Message: "advertisement name already in use by this account"}, nil
		},
		list: func(_ context.Context, params api.ListAdvertisementsParams) (api.ListAdvertisementsRes, error) {
			listed = true
			if !params.Status.Set || params.Status.Value != api.AdvertisementStatusActive {
				t.Fatalf("expected active status lookup, got %#v", params.Status)
			}
			existing := apiAdvertisement("adv_bcdefghijklm", "gateway", now)
			response := api.ListAdvertisementsResponse{*existing}
			return &response, nil
		},
	}

	got, err := ensurePublished(context.Background(), fake, PublishSpec{
		Name:              "gateway",
		Description:       "new description ignored on conflict",
		Capabilities:      []Capability{{Name: "mcp-tools"}},
		WorkgroupScopeIDs: []string{"wg_abcdefghijkl"},
	})
	if err != nil {
		t.Fatalf("ensure published: %v", err)
	}
	if !listed {
		t.Fatal("expected conflict path to list advertisements")
	}
	if got.ID != "adv_bcdefghijklm" || got.Description != "published description" {
		t.Fatalf("expected existing record unchanged, got %#v", got)
	}
}

func TestEnsurePublishedConflictNotFoundReturnsConflict(t *testing.T) {
	fake := &fakeController{
		publish: func(context.Context, *api.PublishAdvertisementRequest) (api.PublishAdvertisementRes, error) {
			return &api.PublishAdvertisementConflict{Code: "name_in_use", Message: "name in use"}, nil
		},
		list: func(context.Context, api.ListAdvertisementsParams) (api.ListAdvertisementsRes, error) {
			response := api.ListAdvertisementsResponse{}
			return &response, nil
		},
	}

	_, err := ensurePublished(context.Background(), fake, validSpec())
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestLookupAndList(t *testing.T) {
	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	fake := &fakeController{
		list: func(_ context.Context, params api.ListAdvertisementsParams) (api.ListAdvertisementsRes, error) {
			if params.Status.Set {
				t.Fatalf("expected no status filter, got %#v", params.Status)
			}
			response := api.ListAdvertisementsResponse{*apiAdvertisement("adv_abcdefghijkl", "Gateway", now)}
			return &response, nil
		},
	}

	got, err := lookup(context.Background(), fake, "gateway")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if got.ID != "adv_abcdefghijkl" {
		t.Fatalf("unexpected lookup result %#v", got)
	}
}

func TestLookupNotFound(t *testing.T) {
	fake := &fakeController{
		list: func(context.Context, api.ListAdvertisementsParams) (api.ListAdvertisementsRes, error) {
			response := api.ListAdvertisementsResponse{}
			return &response, nil
		},
	}

	_, err := lookup(context.Background(), fake, "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestRetractIdempotent(t *testing.T) {
	tests := []struct {
		name api.RetractAdvertisementRes
	}{
		{name: &api.RetractAdvertisementNoContent{}},
		{name: &api.RetractAdvertisementNotFound{Code: "not_found", Message: "advertisement not found"}},
	}

	for _, tt := range tests {
		fake := &fakeController{
			retract: func(_ context.Context, params api.RetractAdvertisementParams) (api.RetractAdvertisementRes, error) {
				if params.AdvertisementId != "adv_abcdefghijkl" {
					t.Fatalf("unexpected advertisement id %q", params.AdvertisementId)
				}
				return tt.name, nil
			},
		}
		if err := retract(context.Background(), fake, "adv_abcdefghijkl"); err != nil {
			t.Fatalf("expected nil retract error, got %v", err)
		}
	}
}

func TestPublishErrorMappings(t *testing.T) {
	tests := []struct {
		name string
		res  api.PublishAdvertisementRes
		err  error
		want error
	}{
		{name: "bad request", res: &api.PublishAdvertisementBadRequest{Code: "invalid_request", Message: "bad"}, want: ErrBadRequest},
		{name: "unauthorized", res: &api.PublishAdvertisementUnauthorized{Code: "unauthorized", Message: "unauthorized"}, want: ErrUnauthorized},
		{name: "forbidden", res: &api.PublishAdvertisementForbidden{Code: "forbidden", Message: "forbidden"}, want: ErrForbidden},
		{name: "internal", res: &api.PublishAdvertisementInternalServerError{Code: "internal_error", Message: "internal"}, want: ErrTransient},
		{name: "transport", err: fmt.Errorf("dial failed"), want: ErrTransient},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeController{
				publish: func(context.Context, *api.PublishAdvertisementRequest) (api.PublishAdvertisementRes, error) {
					return tt.res, tt.err
				},
			}
			_, err := ensurePublished(context.Background(), fake, validSpec())
			if !errors.Is(err, tt.want) {
				t.Fatalf("expected %v, got %v", tt.want, err)
			}
		})
	}
}

func TestListErrorMappings(t *testing.T) {
	tests := []struct {
		name string
		res  api.ListAdvertisementsRes
		err  error
		want error
	}{
		{name: "unauthorized", res: &api.ListAdvertisementsUnauthorized{Code: "unauthorized", Message: "unauthorized"}, want: ErrUnauthorized},
		{name: "internal", res: &api.ListAdvertisementsInternalServerError{Code: "internal_error", Message: "internal"}, want: ErrTransient},
		{name: "transport", err: fmt.Errorf("dial failed"), want: ErrTransient},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeController{
				list: func(context.Context, api.ListAdvertisementsParams) (api.ListAdvertisementsRes, error) {
					return tt.res, tt.err
				},
			}
			_, err := list(context.Background(), fake, "")
			if !errors.Is(err, tt.want) {
				t.Fatalf("expected %v, got %v", tt.want, err)
			}
		})
	}
}

func TestRetractErrorMappings(t *testing.T) {
	tests := []struct {
		name string
		res  api.RetractAdvertisementRes
		err  error
		want error
	}{
		{name: "unauthorized", res: &api.RetractAdvertisementUnauthorized{Code: "unauthorized", Message: "unauthorized"}, want: ErrUnauthorized},
		{name: "forbidden", res: &api.RetractAdvertisementForbidden{Code: "forbidden", Message: "forbidden"}, want: ErrForbidden},
		{name: "internal", res: &api.RetractAdvertisementInternalServerError{Code: "internal_error", Message: "internal"}, want: ErrTransient},
		{name: "transport", err: fmt.Errorf("dial failed"), want: ErrTransient},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeController{
				retract: func(context.Context, api.RetractAdvertisementParams) (api.RetractAdvertisementRes, error) {
					return tt.res, tt.err
				},
			}
			err := retract(context.Background(), fake, "adv_abcdefghijkl")
			if !errors.Is(err, tt.want) {
				t.Fatalf("expected %v, got %v", tt.want, err)
			}
		})
	}
}

func TestValidationErrorsAreBadRequest(t *testing.T) {
	tests := []struct {
		name string
		spec PublishSpec
	}{
		{name: "missing name", spec: PublishSpec{Capabilities: []Capability{{Name: "x"}}, WorkgroupScopeIDs: []string{"wg_abcdefghijkl"}}},
		{name: "missing capabilities", spec: PublishSpec{Name: "gateway", WorkgroupScopeIDs: []string{"wg_abcdefghijkl"}}},
		{name: "invalid workgroup", spec: PublishSpec{Name: "gateway", Capabilities: []Capability{{Name: "x"}}, WorkgroupScopeIDs: []string{"bad"}}},
		{name: "invalid contract", spec: PublishSpec{Name: "gateway", Capabilities: []Capability{{Name: "x"}}, WorkgroupScopeIDs: []string{"wg_abcdefghijkl"}, ContractID: "bad"}},
		{name: "custom missing pattern", spec: PublishSpec{Name: "gateway", Capabilities: []Capability{{Name: "x"}}, WorkgroupScopeIDs: []string{"wg_abcdefghijkl"}, InteractionPatterns: []InteractionPattern{{Kind: InteractionCustom}}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSpec(normalizeSpec(tt.spec))
			if !errors.Is(err, ErrBadRequest) {
				t.Fatalf("expected ErrBadRequest, got %v", err)
			}
		})
	}
}

func validSpec() PublishSpec {
	return PublishSpec{
		Name:              "gateway",
		Capabilities:      []Capability{{Name: "mcp-tools"}},
		WorkgroupScopeIDs: []string{"wg_abcdefghijkl"},
	}
}

func apiAdvertisement(id, name string, now time.Time) *api.Advertisement {
	ad := &api.Advertisement{
		ID:                  id,
		OrganizationId:      "org_abcdefghijkl",
		OrganizationName:    "Gateway Services",
		AccountId:           "ac_abcdefghijkl",
		Name:                name,
		Capabilities:        []api.AdvertisementCapability{{Name: "llm-routing"}},
		InteractionPatterns: []api.AdvertisementInteractionPattern{{Kind: api.AdvertisementInteractionPatternKindRequestResponse}},
		WorkgroupScopes:     []string{"wg_abcdefghijkl"},
		SchemaVersion:       1,
		Status:              api.AdvertisementStatusActive,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	ad.Description.SetTo("published description")
	ad.TunnelMode.SetTo(api.AdvertisementTunnelModeTCP)
	ad.ContractId.SetTo("con_abcdefghijkl")
	return ad
}
