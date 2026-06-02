package openapi

import "testing"

func TestCoerceActiveMembersRenewalDetailRows(t *testing.T) {
	rows := []orderedRow{
		{
			columns: []string{"contact_id", "venue_id", "years", "months", "online", "active", "status"},
			values: map[string]any{
				"contact_id": "38791",
				"venue_id":   "138",
				"years":      "3.4",
				"months":     "40",
				"online":     "0",
				"active":     "1",
				"status":     "Member",
			},
		},
	}

	coerced := coerceActiveMembersRenewalDetailRows(rows)
	got := coerced[0].values

	if value, ok := got["contact_id"].(int); !ok || value != 38791 {
		t.Fatalf("contact_id type/value mismatch: %#v", got["contact_id"])
	}
	if value, ok := got["venue_id"].(int); !ok || value != 138 {
		t.Fatalf("venue_id type/value mismatch: %#v", got["venue_id"])
	}
	if value, ok := got["years"].(float64); !ok || value != 3.4 {
		t.Fatalf("years type/value mismatch: %#v", got["years"])
	}
	if value, ok := got["months"].(int); !ok || value != 40 {
		t.Fatalf("months type/value mismatch: %#v", got["months"])
	}
	if value, ok := got["online"].(bool); !ok || value {
		t.Fatalf("online type/value mismatch: %#v", got["online"])
	}
	if value, ok := got["active"].(bool); !ok || !value {
		t.Fatalf("active type/value mismatch: %#v", got["active"])
	}
	if value, ok := got["status"].(string); !ok || value != "Member" {
		t.Fatalf("status type/value mismatch: %#v", got["status"])
	}
}
