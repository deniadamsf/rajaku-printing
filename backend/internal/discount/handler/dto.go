package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rajaku-printing/backend/internal/discount/service"
)

// ---- Request DTOs ----

type createDiscountRequest struct {
	Code              string     `json:"code"                binding:"required,max=30"`
	Name              string     `json:"name"                binding:"required,max=150"`
	Type              string     `json:"type"                binding:"required,oneof=percent nominal"`
	ValuePercent      *float64   `json:"value_percent"`
	ValueAmount       *int64     `json:"value_amount"`
	MaxDiscountAmount *int64     `json:"max_discount_amount"`
	MinSubtotal       int64      `json:"min_subtotal"        binding:"omitempty,gte=0"`
	StartsAt          *time.Time `json:"starts_at"`
	EndsAt            *time.Time `json:"ends_at"`
	Quota             *int       `json:"quota"`
	ChannelScope      string     `json:"channel_scope"       binding:"omitempty,oneof=all online pos"`
	IsActive          *bool      `json:"is_active"`
}

type deleteDiscountRequest struct {
	Reason string `json:"reason" binding:"required,min=3,max=500"`
}

// parseUpdateInput turns a raw JSON object (map[string]json.RawMessage) into
// a service.UpdateInput, distinguishing "key absent" (leave untouched) from
// "key present with null" (explicit clear — only meaningful for
// MaxDiscountAmount/StartsAt/EndsAt/Quota, the genuinely nullable business
// fields) from "key present with a value" (set). Plain map[string]any binding
// via c.ShouldBindJSON can't make that distinction (encoding/json collapses
// both "absent" and "null" to the zero value), hence the manual RawMessage
// walk here — this is HTTP-layer request parsing, not business logic (§22:
// handler may bind/validate input, it just can't touch *gorm.DB).
func parseUpdateInput(raw map[string]json.RawMessage) (*service.UpdateInput, error) {
	in := &service.UpdateInput{}

	if v, ok := raw["code"]; ok {
		if isJSONNull(v) {
			return nil, fmt.Errorf("code tidak boleh null")
		}
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			return nil, fmt.Errorf("code: %w", err)
		}
		in.Code = &s
	}
	if v, ok := raw["name"]; ok {
		if isJSONNull(v) {
			return nil, fmt.Errorf("name tidak boleh null")
		}
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			return nil, fmt.Errorf("name: %w", err)
		}
		in.Name = &s
	}
	if v, ok := raw["type"]; ok {
		if isJSONNull(v) {
			return nil, fmt.Errorf("type tidak boleh null")
		}
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			return nil, fmt.Errorf("type: %w", err)
		}
		in.Type = &s
	}
	if v, ok := raw["value_percent"]; ok {
		if isJSONNull(v) {
			return nil, fmt.Errorf("value_percent tidak boleh null (ganti type kalau ingin lepas dari persen)")
		}
		var f float64
		if err := json.Unmarshal(v, &f); err != nil {
			return nil, fmt.Errorf("value_percent: %w", err)
		}
		in.ValuePercent = &f
	}
	if v, ok := raw["value_amount"]; ok {
		if isJSONNull(v) {
			return nil, fmt.Errorf("value_amount tidak boleh null (ganti type kalau ingin lepas dari nominal)")
		}
		var n int64
		if err := json.Unmarshal(v, &n); err != nil {
			return nil, fmt.Errorf("value_amount: %w", err)
		}
		in.ValueAmount = &n
	}
	if v, ok := raw["min_subtotal"]; ok {
		if isJSONNull(v) {
			return nil, fmt.Errorf("min_subtotal tidak boleh null")
		}
		var n int64
		if err := json.Unmarshal(v, &n); err != nil {
			return nil, fmt.Errorf("min_subtotal: %w", err)
		}
		in.MinSubtotal = &n
	}
	if v, ok := raw["channel_scope"]; ok {
		if isJSONNull(v) {
			return nil, fmt.Errorf("channel_scope tidak boleh null")
		}
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			return nil, fmt.Errorf("channel_scope: %w", err)
		}
		in.ChannelScope = &s
	}
	if v, ok := raw["is_active"]; ok {
		if isJSONNull(v) {
			return nil, fmt.Errorf("is_active tidak boleh null")
		}
		var b bool
		if err := json.Unmarshal(v, &b); err != nil {
			return nil, fmt.Errorf("is_active: %w", err)
		}
		in.IsActive = &b
	}

	if v, ok := raw["max_discount_amount"]; ok {
		if isJSONNull(v) {
			in.ClearMaxDiscountAmount = true
		} else {
			var n int64
			if err := json.Unmarshal(v, &n); err != nil {
				return nil, fmt.Errorf("max_discount_amount: %w", err)
			}
			in.MaxDiscountAmount = &n
		}
	}
	if v, ok := raw["starts_at"]; ok {
		if isJSONNull(v) {
			in.ClearStartsAt = true
		} else {
			var t time.Time
			if err := json.Unmarshal(v, &t); err != nil {
				return nil, fmt.Errorf("starts_at: %w", err)
			}
			in.StartsAt = &t
		}
	}
	if v, ok := raw["ends_at"]; ok {
		if isJSONNull(v) {
			in.ClearEndsAt = true
		} else {
			var t time.Time
			if err := json.Unmarshal(v, &t); err != nil {
				return nil, fmt.Errorf("ends_at: %w", err)
			}
			in.EndsAt = &t
		}
	}
	if v, ok := raw["quota"]; ok {
		if isJSONNull(v) {
			in.ClearQuota = true
		} else {
			var n int
			if err := json.Unmarshal(v, &n); err != nil {
				return nil, fmt.Errorf("quota: %w", err)
			}
			in.Quota = &n
		}
	}

	return in, nil
}

func isJSONNull(v json.RawMessage) bool {
	return string(bytes.TrimSpace(v)) == "null"
}
