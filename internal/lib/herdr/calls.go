package herdr

import "context"

// Ping probes server liveness and reads its version, protocol, and capabilities.
func (c Client) Ping(ctx context.Context) (PongResult, error) {
	var r PongResult
	if err := c.Call(ctx, "ping", nil, &r); err != nil {
		return PongResult{}, err
	}
	if r.Type != "pong" {
		return PongResult{}, &Error{Code: "protocol_error", Message: "unexpected ping result " + r.Type, Uncertain: true}
	}
	return r, nil
}

// Caller sends one Herdr request; Client and operation clients that locate
// failures by phase both satisfy it.
type Caller interface {
	Call(ctx context.Context, method string, params any, result any) error
}

// IntegrationList reads every known integration target and its install state.
func (c Client) IntegrationList(ctx context.Context) (IntegrationListResult, error) {
	return IntegrationList(ctx, c)
}

// IntegrationList sends integration.list through c, returning c's error
// unchanged and rejecting an incomplete result as an uncertain protocol error.
func IntegrationList(ctx context.Context, c Caller) (IntegrationListResult, error) {
	var r IntegrationListResult
	if err := c.Call(ctx, "integration.list", nil, &r); err != nil {
		return IntegrationListResult{}, err
	}
	if r.Type != "integration_list" || r.Integrations == nil {
		return IntegrationListResult{}, &Error{Code: "protocol_error", Message: "incomplete integration.list result", Uncertain: true}
	}
	return r, nil
}
