package search

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
)

// RemoveFindings never deletes an index or trusts IDs without a tenant filter.
func (c *Client) RemoveFindings(ctx context.Context, tenant uuid.UUID, ids []uuid.UUID) error {
	if tenant == uuid.Nil || len(ids) > 256 {
		return fmt.Errorf("invalid demo finding cleanup batch")
	}
	if len(ids) == 0 {
		return nil
	}
	body := map[string]any{"query": map[string]any{"bool": map[string]any{"filter": []any{map[string]any{"term": map[string]any{"tenant_id": tenant.String()}}, map[string]any{"ids": map[string]any{"values": ids}}}}}}
	raw, code, err := c.do(ctx, "POST", "/"+c.Index+"/_delete_by_query?conflicts=proceed&refresh=true", body)
	if err != nil {
		return err
	}
	if code == 404 {
		var result struct {
			Error struct {
				Type string `json:"type"`
			} `json:"error"`
		}
		if json.Unmarshal(raw, &result) == nil && result.Error.Type == "index_not_found_exception" {
			return nil
		}
	}
	if code >= 300 {
		return fmt.Errorf("search cleanup returned status %d", code)
	}
	var result struct {
		TimedOut  bool              `json:"timed_out"`
		Failures  []json.RawMessage `json:"failures"`
		Conflicts int               `json:"version_conflicts"`
		Total     *int64            `json:"total"`
		Deleted   *int64            `json:"deleted"`
	}
	if json.Unmarshal(raw, &result) != nil || result.TimedOut || len(result.Failures) > 0 || result.Conflicts > 0 || result.Total == nil || result.Deleted == nil || *result.Total < 0 || *result.Deleted != *result.Total {
		return fmt.Errorf("search cleanup incomplete; retry required")
	}
	return nil
}
