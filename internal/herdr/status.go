package herdr

import (
	"context"
	"errors"
	"fmt"
)

// Protocol returns the socket protocol version the Herdr executable speaks.
func (c *Client) Protocol(ctx context.Context) (int, error) {
	var response struct {
		Client struct {
			Protocol *int `json:"protocol"`
		} `json:"client"`
	}

	if err := c.runJSON(ctx, &response, "status", "--json"); err != nil {
		return 0, fmt.Errorf("read Herdr status: %w", err)
	}
	if response.Client.Protocol == nil {
		return 0, errors.New("read Herdr status: missing client protocol")
	}

	return *response.Client.Protocol, nil
}
