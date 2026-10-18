package client

import (
	"context"
	"encoding/json"
	"net/url"
	"sort"
)

// ConfigurationRequest uses the normal authenticated transport and envelope
// handling for provider-owned declarative API contracts.
func (c *Client) ConfigurationRequest(ctx context.Context, method, path string, body any) (json.RawMessage, error) {
	return c.doRequest(ctx, method, path, body)
}
func (c *Client) ConfigurationList(ctx context.Context, path string, params url.Values) ([]json.RawMessage, error) {
	if params == nil {
		params = url.Values{}
	}
	if path == "/api/v1/commands" && params.Get("resource_type") == "" {
		raw, err := c.doRequest(ctx, "GET", path+"/counts", nil)
		if err != nil {
			return nil, err
		}
		var counts map[string]json.RawMessage
		if err = json.Unmarshal(raw, &counts); err != nil {
			return nil, err
		}
		kinds := make([]string, 0, len(counts))
		for kind := range counts {
			kinds = append(kinds, kind)
		}
		sort.Strings(kinds)
		var result []json.RawMessage
		for _, kind := range kinds {
			p := url.Values{"resource_type": {kind}}
			rows, e := c.ConfigurationList(ctx, path, p)
			if e != nil {
				return nil, e
			}
			result = append(result, rows...)
		}
		return result, nil
	}
	// The CA catalog caps page size at 100; this is accepted by every managed catalog.
	params.Set("size", "100")
	return paginateJSON[json.RawMessage](ctx, c, path, params, "configuration objects")
}
