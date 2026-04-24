package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/openziti/agora/internal/api"
	"github.com/spf13/cobra"
)

var advertiseCmd = &cobra.Command{
	Use:   "advertise",
	Short: "Manage advertisements published by the current account",
}

func init() {
	rootCmd.AddCommand(advertiseCmd)
}

// resolveContractID resolves a name-or-id token to a con_... ID by
// listing the caller's own contracts. If the token is already a
// con_... ID it is returned as-is. The sentinel "-none-" returns the
// empty string so callers can clear the contract reference on update.
func resolveContractID(client *api.Client, token string) string {
	token = strings.TrimSpace(token)
	if token == "-none-" || token == "" {
		return ""
	}
	if strings.HasPrefix(token, "con_") {
		return token
	}
	res, err := client.ListContracts(context.Background())
	panicIfErr(err)
	listing, ok := res.(*api.ListContractsResponse)
	if !ok {
		panic(fmt.Sprintf("unexpected list contracts response: %T", res))
	}
	matches := make([]api.Contract, 0)
	for _, c := range *listing {
		if strings.EqualFold(c.Name, token) {
			matches = append(matches, c)
		}
	}
	switch len(matches) {
	case 0:
		panic(fmt.Sprintf("no contract matches name or id '%s'", token))
	case 1:
		return matches[0].ID
	default:
		ids := make([]string, len(matches))
		for i, c := range matches {
			ids[i] = c.ID
		}
		panic(fmt.Sprintf("multiple contracts match name '%s'; specify the id explicitly: %s", token, strings.Join(ids, ", ")))
	}
}

// resolveAdvertisementID resolves a name-or-id token to an adv_... ID
// by listing the caller's own advertisements. If the token is already
// an adv_... ID it is returned as-is.
func resolveAdvertisementID(client *api.Client, token string) string {
	token = strings.TrimSpace(token)
	if strings.HasPrefix(token, "adv_") {
		return token
	}
	res, err := client.ListAdvertisements(context.Background(), api.ListAdvertisementsParams{})
	panicIfErr(err)
	listing, ok := res.(*api.ListAdvertisementsResponse)
	if !ok {
		panic(fmt.Sprintf("unexpected list advertisements response: %T", res))
	}
	matches := make([]api.Advertisement, 0)
	for _, ad := range *listing {
		if strings.EqualFold(ad.Name, token) {
			matches = append(matches, ad)
		}
	}
	switch len(matches) {
	case 0:
		panic(fmt.Sprintf("no advertisement matches name or id '%s'", token))
	case 1:
		return matches[0].ID
	default:
		ids := make([]string, len(matches))
		for i, ad := range matches {
			ids[i] = ad.ID
		}
		panic(fmt.Sprintf("multiple advertisements match name '%s'; specify the id explicitly: %s", token, strings.Join(ids, ", ")))
	}
}

// parseCapability parses a CLI --capability value of the form
// "name[=description][:k1=v1,k2=v2]" into an api.AdvertisementCapability.
func parseCapability(raw string) api.AdvertisementCapability {
	cap := api.AdvertisementCapability{}
	rest := raw
	// metadata after first ":" (only first; metadata values themselves don't contain ":")
	if idx := strings.Index(rest, ":"); idx >= 0 {
		metaRaw := rest[idx+1:]
		rest = rest[:idx]
		md := api.AdvertisementCapabilityMetadata{}
		for _, pair := range strings.Split(metaRaw, ",") {
			pair = strings.TrimSpace(pair)
			if pair == "" {
				continue
			}
			eq := strings.Index(pair, "=")
			if eq < 0 {
				panic(fmt.Sprintf("metadata pair %q must be key=value", pair))
			}
			md[strings.TrimSpace(pair[:eq])] = strings.TrimSpace(pair[eq+1:])
		}
		if len(md) > 0 {
			cap.Metadata.SetTo(md)
		}
	}
	if eq := strings.Index(rest, "="); eq >= 0 {
		cap.Name = strings.TrimSpace(rest[:eq])
		cap.Description.SetTo(strings.TrimSpace(rest[eq+1:]))
	} else {
		cap.Name = strings.TrimSpace(rest)
	}
	if cap.Name == "" {
		panic(fmt.Sprintf("invalid --capability %q: name is required", raw))
	}
	return cap
}

// parseInteractionPattern parses a CLI --interaction value into an
// api.AdvertisementInteractionPattern. Accepts a bare kind string
// (e.g. "request-response") or "custom:<pattern>".
func parseInteractionPattern(raw string) api.AdvertisementInteractionPattern {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "custom:") {
		return api.AdvertisementInteractionPattern{
			Kind:          api.AdvertisementInteractionPatternKindCustom,
			CustomPattern: api.NewOptString(strings.TrimSpace(strings.TrimPrefix(raw, "custom:"))),
		}
	}
	switch raw {
	case "request-response", "stream", "broadcast":
		return api.AdvertisementInteractionPattern{Kind: api.AdvertisementInteractionPatternKind(raw)}
	default:
		panic(fmt.Sprintf("invalid --interaction value %q (expected request-response, stream, broadcast, or custom:<pattern>)", raw))
	}
}
