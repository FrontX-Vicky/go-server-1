package dynamicapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// QueryRequest represents the dynamic query payload coming from the frontend.
type QueryRequest struct {
	Table        string         `json:"table"`
	Select       []string       `json:"select"`
	Filters      *FilterGroup   `json:"filters"`
	Sort         []SortSpec     `json:"sort"`
	Page         PageSpec       `json:"page"`
	IncludeTotal bool           `json:"includeTotal"`
	Distinct     bool           `json:"distinct"`
	GroupBy      []string       `json:"groupBy"`
	Having       *FilterGroup   `json:"having"`
	Search       *SearchSpec    `json:"search"`
	Meta         map[string]any `json:"meta"`
}

// Normalize trims and applies defaults on the incoming payload.
func (qr *QueryRequest) Normalize() error {
	qr.Table = strings.TrimSpace(qr.Table)
	if qr.Table == "" {
		return errors.New("table is required")
	}

	if qr.Page.Number <= 0 {
		qr.Page.Number = 1
	}
	if qr.Page.Size <= 0 {
		qr.Page.Size = 25
	} else if qr.Page.Size > 500 {
		qr.Page.Size = 500
	}

	qr.Select = trimSlice(qr.Select)
	qr.GroupBy = trimSlice(qr.GroupBy)

	for i := range qr.Sort {
		qr.Sort[i].Field = strings.TrimSpace(qr.Sort[i].Field)
		qr.Sort[i].Dir = strings.ToLower(strings.TrimSpace(qr.Sort[i].Dir))
		if qr.Sort[i].Dir == "" {
			qr.Sort[i].Dir = "asc"
		}
		qr.Sort[i].Nulls = strings.ToLower(strings.TrimSpace(qr.Sort[i].Nulls))
	}

	if qr.Search != nil {
		qr.Search.Term = strings.TrimSpace(qr.Search.Term)
		qr.Search.Fields = trimSlice(qr.Search.Fields)
	}

	if qr.Filters != nil {
		qr.Filters.normalize()
	}
	if qr.Having != nil {
		qr.Having.normalize()
	}

	return nil
}

func trimSlice(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if v := strings.TrimSpace(item); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// SortSpec represents order by instructions.
type SortSpec struct {
	Field string `json:"field"`
	Dir   string `json:"dir"`
	Nulls string `json:"nulls"`
}

// PageSpec captures pagination details.
type PageSpec struct {
	Number int `json:"number"`
	Size   int `json:"size"`
}

// SearchSpec allows full-text like searches across multiple fields.
type SearchSpec struct {
	Term   string   `json:"term"`
	Fields []string `json:"fields"`
}

// FilterGroup captures a logical grouping of filter expressions.
type FilterGroup struct {
	Logic      string             `json:"logic"`
	Conditions []FilterExpression `json:"conditions"`
}

// FilterExpression is either a nested group or a leaf condition.
type FilterExpression struct {
	Group     *FilterGroup
	Condition *FilterCondition
}

// FilterCondition represents an operator applied to a single field.
type FilterCondition struct {
	Field         string `json:"field"`
	Op            string `json:"op"`
	Value         any    `json:"value"`
	Timezone      string `json:"timezone"`
	CaseSensitive *bool  `json:"caseSensitive"`
}

func (expr *FilterExpression) UnmarshalJSON(data []byte) error {
	var probe struct {
		Logic *string `json:"logic"`
		Field *string `json:"field"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return err
	}

	switch {
	case probe.Logic != nil:
		var grp FilterGroup
		if err := json.Unmarshal(data, &grp); err != nil {
			return err
		}
		expr.Group = &grp
	case probe.Field != nil:
		var cond FilterCondition
		if err := json.Unmarshal(data, &cond); err != nil {
			return err
		}
		expr.Condition = &cond
	default:
		return fmt.Errorf("invalid filter expression: %s", string(data))
	}
	return nil
}

func (fg *FilterGroup) normalize() {
	if fg == nil {
		return
	}
	fg.Logic = strings.ToLower(strings.TrimSpace(fg.Logic))
	if fg.Logic != "or" {
		fg.Logic = "and"
	}
	for _, expr := range fg.Conditions {
		if expr.Group != nil {
			expr.Group.normalize()
		}
	}
}
