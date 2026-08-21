package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// FieldChange records the before/after of a single field in an admin audit
// row — `From`/`To` are `any` because the fields touched by different
// actions have different Go types (string, *int64, state.Status, ...); the
// caller is responsible for putting comparable/JSON-marshalable values here.
type FieldChange struct {
	From any `json:"from"`
	To   any `json:"to"`
}

// ChangeSet is the GORM (de)serializer for the jsonb `changes` column —
// {"field": {"from": x, "to": y}} per field that ACTUALLY changed (not a
// full snapshot). Mirrors notification.JSONPayload's Value/Scan pattern
// (internal/notification/model/notification_job.go) so we don't pull in
// gorm.io/datatypes as a new dependency just for this.
type ChangeSet map[string]FieldChange

func (c ChangeSet) Value() (driver.Value, error) {
	if c == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(c)
}

func (c *ChangeSet) Scan(src any) error {
	if src == nil {
		*c = ChangeSet{}
		return nil
	}
	var b []byte
	switch v := src.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		return errors.New("order.ChangeSet: unsupported scan type")
	}
	if len(b) == 0 {
		*c = ChangeSet{}
		return nil
	}
	return json.Unmarshal(b, c)
}

// AdminAuditLog records ONE super-admin action against an order (§ super
// admin order tools): edit data pesanan, override status, or soft-delete.
// Every write to this table happens in the SAME database transaction as the
// mutation it describes (see repository.insertAuditLog, called from inside
// OrderRepository.UpdateFields / OverrideStatus / SoftDelete) — a change is
// never persisted without its audit row, or vice versa.
//
// EntityType is always "order" today; the column is deliberately generic
// (not a FK to orders.id) so this table can record audit entries for other
// entity types later without a schema change.
type AdminAuditLog struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ActorUserID uuid.UUID `gorm:"type:uuid;not null"                             json:"actor_user_id"`
	Action      string    `gorm:"size:50;not null"                               json:"action"`
	EntityType  string    `gorm:"size:50;not null"                               json:"entity_type"`
	EntityID    uuid.UUID `gorm:"type:uuid;not null"                             json:"entity_id"`
	EntityLabel *string   `gorm:"size:50"                                        json:"entity_label,omitempty"`
	Changes     ChangeSet `gorm:"type:jsonb"                                     json:"changes,omitempty"`
	Reason      string    `gorm:"not null"                                       json:"reason"`
	CreatedAt   time.Time `gorm:"not null;default:now()"                         json:"created_at"`
}

func (AdminAuditLog) TableName() string { return "admin_audit_log" }
