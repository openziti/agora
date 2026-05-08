// Package catalog provides public SDK helpers for publishing and
// managing an agent's Layer 2 catalog advertisements.
//
// The package intentionally exposes SDK-native types rather than the
// controller's generated OpenAPI types. That keeps external modules
// from depending on Agora's internal API package while still allowing
// agents to publish, look up, list, and retract their own
// advertisements through an authenticated *agent.Agent.
package catalog
