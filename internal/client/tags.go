package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// ---------------------------------------------------------------------------
// Tag API models
// ---------------------------------------------------------------------------

type TagCreateInput struct {
	Name        string  `json:"name"`
	Color       string  `json:"color,omitempty"` // backend defaults "grey"
	Description *string `json:"description,omitempty"`
}

type TagUpdateInput struct {
	Name        *string `json:"name,omitempty"`
	Color       *string `json:"color,omitempty"` // color min_length 1: never send null
	Description *string `json:"description"`     // no omitempty: explicit null clears
}

type TagResponse struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	Color       string  `json:"color"`
	Description *string `json:"description"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

// TagListItem is the List endpoint shape (resource_count, no timestamps).
type TagListItem struct {
	ID            int64   `json:"id"`
	Name          string  `json:"name"`
	Color         string  `json:"color"`
	Description   *string `json:"description"`
	ResourceCount int64   `json:"resource_count"`
}

type TagListFilter struct {
	Search string
}

// TagAssignmentInput is the resource_ids assignment body for the whole-list PUT.
type TagAssignmentInput struct {
	ResourceIDs []int64 `json:"resource_ids"`
}

func (c *Client) CreateTag(ctx context.Context, input TagCreateInput) (*TagResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPost, "/api/v1/tags", input)
	if err != nil {
		return nil, err
	}
	var tag TagResponse
	if err := json.Unmarshal(data, &tag); err != nil {
		return nil, fmt.Errorf("decoding tag: %w", err)
	}
	return &tag, nil
}

func (c *Client) GetTag(ctx context.Context, id int64) (*TagResponse, error) {
	data, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v1/tags/%d", id), nil)
	if err != nil {
		return nil, err
	}
	var tag TagResponse
	if err := json.Unmarshal(data, &tag); err != nil {
		return nil, fmt.Errorf("decoding tag: %w", err)
	}
	return &tag, nil
}

func (c *Client) UpdateTag(ctx context.Context, id int64, input TagUpdateInput) (*TagResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPatch, fmt.Sprintf("/api/v1/tags/%d", id), input)
	if err != nil {
		return nil, err
	}
	var tag TagResponse
	if err := json.Unmarshal(data, &tag); err != nil {
		return nil, fmt.Errorf("decoding tag: %w", err)
	}
	return &tag, nil
}

func (c *Client) DeleteTag(ctx context.Context, id int64) error {
	_, err := c.doRequest(ctx, http.MethodDelete, fmt.Sprintf("/api/v1/tags/%d", id), nil)
	return err
}

func (c *Client) ListTags(ctx context.Context, filter TagListFilter) ([]TagListItem, error) {
	params := url.Values{}
	params.Set("size", "10000")
	if filter.Search != "" {
		params.Set("search", filter.Search)
	}
	return paginateJSON[TagListItem](ctx, c, "/api/v1/tags", params, "tags")
}

// SetTagAssignments replaces the whole set of resource ids assigned to the tag.
// An empty slice clears all assignments (sends [], not null).
func (c *Client) SetTagAssignments(ctx context.Context, id int64, resourceIDs []int64) error {
	if resourceIDs == nil {
		resourceIDs = []int64{}
	}
	_, err := c.doRequest(ctx, http.MethodPut, fmt.Sprintf("/api/v1/tags/%d/assignments", id), TagAssignmentInput{ResourceIDs: resourceIDs})
	return err
}

func (c *Client) GetTagResourceIDs(ctx context.Context, id int64) ([]int64, error) {
	data, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v1/tags/%d/resource-ids", id), nil)
	if err != nil {
		return nil, err
	}
	var ids []int64
	if err := json.Unmarshal(data, &ids); err != nil {
		return nil, fmt.Errorf("decoding tag resource ids: %w", err)
	}
	return ids, nil
}

// ResourceTagAssignment is the body of PUT /resources/{id}/tags — the
// resource-centric counterpart to TagAssignmentInput. It replaces the resource's
// tag set exactly (an empty slice clears all tags).
type ResourceTagAssignment struct {
	TagIDs []int64 `json:"tag_ids"`
}

// SetResourceTags replaces the whole set of tag ids assigned to a resource via
// PUT /api/v1/resources/{id}/tags. An empty slice clears all tags (sends [], not
// null). This is the resource-centric ownership direction (vs SetTagAssignments,
// which is tag-centric); own each tag<->resource edge from one side only.
func (c *Client) SetResourceTags(ctx context.Context, resourceID int64, tagIDs []int64) ([]TagResponse, error) {
	if tagIDs == nil {
		tagIDs = []int64{}
	}
	// PUT returns an empty (NoneType) body, so re-read the authoritative tag set.
	if _, err := c.doRequest(ctx, http.MethodPut, fmt.Sprintf("/api/v1/resources/%d/tags", resourceID), ResourceTagAssignment{TagIDs: tagIDs}); err != nil {
		return nil, err
	}
	return c.GetResourceTags(ctx, resourceID)
}

// GetResourceTags returns the full tag objects assigned to a resource via
// GET /api/v1/resources/{id}/tags.
func (c *Client) GetResourceTags(ctx context.Context, resourceID int64) ([]TagResponse, error) {
	data, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v1/resources/%d/tags", resourceID), nil)
	if err != nil {
		return nil, err
	}
	var tags []TagResponse
	if err := json.Unmarshal(data, &tags); err != nil {
		return nil, fmt.Errorf("decoding resource tags: %w", err)
	}
	return tags, nil
}
