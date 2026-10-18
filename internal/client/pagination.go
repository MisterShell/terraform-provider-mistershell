package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// PaginationMeta describes a MisterShell paginated response.
type PaginationMeta struct {
	Page  int `json:"page"`
	Size  int `json:"size"`
	Total int `json:"total"`
	Pages int `json:"pages"`
}

func paginate[T any](ctx context.Context, fetch func(page int) ([]T, *PaginationMeta, error)) ([]T, error) {
	var all []T
	for page := 1; ; page++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		items, meta, err := fetch(page)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
		if meta == nil || meta.Pages <= page {
			return all, nil
		}
	}
}

func paginateJSON[T any](ctx context.Context, c *Client, path string, params url.Values, description string) ([]T, error) {
	return paginate(ctx, func(page int) ([]T, *PaginationMeta, error) {
		params.Set("page", fmt.Sprintf("%d", page))
		data, meta, err := c.doRequestWithMeta(ctx, http.MethodGet, path+"?"+params.Encode(), nil)
		if err != nil {
			return nil, nil, err
		}
		var items []T
		if err := json.Unmarshal(data, &items); err != nil {
			return nil, nil, fmt.Errorf("decoding %s: %w", description, err)
		}
		return items, meta, nil
	})
}

// SearchPrefix bounds a candidate search for an exact-name lookup to the API limit.
// Callers must still perform exact matching across all returned pages.
func SearchPrefix(value string) string {
	r := []rune(value)
	if len(r) > 100 {
		return string(r[:100])
	}
	return value
}
