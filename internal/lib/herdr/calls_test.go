package herdr

import (
	"bufio"
	"context"
	"errors"
	"net"
	"testing"
)

// reply serves one request on a fresh socket and writes line as the response.
func reply(t *testing.T, line string) string {
	t.Helper()
	return socket(t, func(c net.Conn) {
		bufio.NewReader(c).ReadString('\n')
		c.Write([]byte(line + "\n"))
	})
}

func TestPing(t *testing.T) {
	path := reply(t, `{"id":"fledge","result":{"type":"pong","version":"0.9.1","protocol":22,"capabilities":{"live_handoff":true,"detached_server_daemon":false,"health_check":true,"surface_interest":true,"endpoint_protocol_generation":1}}}`)
	pong, err := (Client{Socket: path}).Ping(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if pong.Type != "pong" || pong.Version != "0.9.1" || pong.Protocol != 22 {
		t.Fatalf("%+v", pong)
	}
	if pong.Capabilities == nil || !pong.Capabilities.LiveHandoff || !pong.Capabilities.HealthCheck || !pong.Capabilities.SurfaceInterest || pong.Capabilities.DetachedServerDaemon {
		t.Fatalf("capabilities %+v", pong.Capabilities)
	}
	if pong.Capabilities.EndpointProtocolGeneration == nil || *pong.Capabilities.EndpointProtocolGeneration != 1 {
		t.Fatalf("endpoint generation %+v", pong.Capabilities.EndpointProtocolGeneration)
	}
}

func TestPingNullCapabilities(t *testing.T) {
	path := reply(t, `{"id":"fledge","result":{"type":"pong","version":"0.9.1","protocol":22,"capabilities":null}}`)
	pong, err := (Client{Socket: path}).Ping(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if pong.Capabilities != nil {
		t.Fatalf("capabilities %+v", pong.Capabilities)
	}
}

func TestPingWrongType(t *testing.T) {
	path := reply(t, `{"id":"fledge","result":{"type":"integration_list","integrations":[]}}`)
	_, err := (Client{Socket: path}).Ping(context.Background())
	var e *Error
	if !errors.As(err, &e) || e.Code != "protocol_error" {
		t.Fatalf("got %#v", err)
	}
}

func TestPingPropagatesServerError(t *testing.T) {
	path := reply(t, `{"id":"fledge","error":{"code":"server_not_running","message":"no server"}}`)
	_, err := (Client{Socket: path}).Ping(context.Background())
	var e *Error
	if !errors.As(err, &e) || e.Code != "server_not_running" {
		t.Fatalf("got %#v", err)
	}
}

func TestIntegrationList(t *testing.T) {
	path := reply(t, `{"id":"fledge","result":{"type":"integration_list","integrations":[{"target":"pi","label":"pi","command":"pi","available":true,"state":"current"},{"target":"cursor","label":"cursor","command":"cursor-agent","available":false,"state":"not_installed"}]}}`)
	list, err := (Client{Socket: path}).IntegrationList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Integrations) != 2 {
		t.Fatalf("%+v", list)
	}
	pi := list.Integrations[0]
	if pi.Target != "pi" || pi.Command != "pi" || !pi.Available || pi.State != "current" {
		t.Fatalf("%+v", pi)
	}
	cursor := list.Integrations[1]
	if cursor.Target != "cursor" || cursor.Command != "cursor-agent" || cursor.Available || cursor.State != "not_installed" {
		t.Fatalf("%+v", cursor)
	}
}

func TestIntegrationListMissingArray(t *testing.T) {
	path := reply(t, `{"id":"fledge","result":{"type":"integration_list"}}`)
	_, err := (Client{Socket: path}).IntegrationList(context.Background())
	var e *Error
	if !errors.As(err, &e) || e.Code != "protocol_error" {
		t.Fatalf("got %#v", err)
	}
}

func TestIntegrationListWrongType(t *testing.T) {
	path := reply(t, `{"id":"fledge","result":{"type":"pong","version":"0.9.1","protocol":22}}`)
	_, err := (Client{Socket: path}).IntegrationList(context.Background())
	var e *Error
	if !errors.As(err, &e) || e.Code != "protocol_error" {
		t.Fatalf("got %#v", err)
	}
}
