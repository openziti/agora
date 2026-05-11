package tunnel

import (
	"net"
	"net/url"
	"strings"
)

func normalizeServeSpec(spec ServeSpec) ServeSpec {
	spec.Name = strings.TrimSpace(spec.Name)
	spec.Mode = Mode(strings.ToLower(strings.TrimSpace(string(spec.Mode))))
	spec.BackendTarget = strings.TrimSpace(spec.BackendTarget)
	grants := make([]string, 0, len(spec.GrantEmails))
	for _, email := range spec.GrantEmails {
		email = strings.TrimSpace(email)
		if email == "" {
			continue
		}
		grants = append(grants, email)
	}
	spec.GrantEmails = grants
	return spec
}

func validateServeSpec(spec ServeSpec) error {
	if spec.Name == "" {
		return invalidSpec("serve name is required")
	}
	if spec.Mode == "" {
		return invalidSpec("serve mode is required")
	}
	switch spec.Mode {
	case ModeHTTP:
		u, err := url.Parse(spec.BackendTarget)
		if err != nil {
			return invalidSpec("http backend target is invalid")
		}
		if u.Scheme == "" || u.Host == "" {
			return invalidSpec("http backend target must include scheme and host")
		}
	case ModeTCP, ModeUDP:
		if _, _, err := net.SplitHostPort(spec.BackendTarget); err != nil {
			return invalidSpec("backend target must be host:port")
		}
	default:
		return unsupportedMode("unsupported tunnel mode '%s'", spec.Mode)
	}
	return nil
}

func normalizeConnectSpec(spec ConnectSpec) ConnectSpec {
	spec.Name = strings.TrimSpace(spec.Name)
	spec.ListenAddress = strings.TrimSpace(spec.ListenAddress)
	return spec
}

func validateConnectSpec(spec ConnectSpec) error {
	if spec.Name == "" {
		return invalidSpec("connect name is required")
	}
	host, port, err := net.SplitHostPort(spec.ListenAddress)
	if err != nil {
		return invalidSpec("listen address must be host:port")
	}
	if strings.TrimSpace(host) == "" || strings.TrimSpace(port) == "" || port == "0" {
		return invalidSpec("listen address must be a concrete host:port")
	}
	return nil
}
