// Package model contains GORM entities for the notification module.
package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

type JobStatus string

const (
	JobPending JobStatus = "pending"
	JobSending JobStatus = "sending"
	JobSent    JobStatus = "sent"
	JobFailed  JobStatus = "failed" // transient — will retry
	JobDead    JobStatus = "dead"   // exhausted retries; needs human
)

// JSONPayload — GORM (de)serializer for the jsonb `payload` column. Kept tiny
// so we don't pull datatypes.JSON as a dependency.
type JSONPayload map[string]any

func (p JSONPayload) Value() (driver.Value, error) {
	if p == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(p)
}

func (p *JSONPayload) Scan(src any) error {
	if src == nil {
		*p = JSONPayload{}
		return nil
	}
	var b []byte
	switch v := src.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		return errors.New("notification.JSONPayload: unsupported scan type")
	}
	if len(b) == 0 {
		*p = JSONPayload{}
		return nil
	}
	return json.Unmarshal(b, p)
}

type NotificationJob struct {
	ID             uuid.UUID   `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Kind           string      `gorm:"size:50;not null"                                json:"kind"`
	RecipientPhone string      `gorm:"size:20;not null"                                json:"recipient_phone"`
	Message        string      `gorm:"type:text;not null"                              json:"message"`
	Payload        JSONPayload `gorm:"type:jsonb;not null;default:'{}'::jsonb"         json:"payload"`

	DedupKey *string `gorm:"size:150;uniqueIndex" json:"dedup_key,omitempty"`

	Status         JobStatus  `gorm:"size:20;not null;default:pending" json:"status"`
	Attempts       int        `gorm:"not null;default:0"               json:"attempts"`
	MaxAttempts    int        `gorm:"not null;default:5"               json:"max_attempts"`
	NextAttemptAt  time.Time  `gorm:"not null;default:now()"           json:"next_attempt_at"`
	LastError      *string    `gorm:"type:text"                        json:"last_error,omitempty"`
	SentAt         *time.Time `                                        json:"sent_at,omitempty"`

	OrderID *uuid.UUID `gorm:"type:uuid" json:"order_id,omitempty"`

	CreatedAt time.Time `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt time.Time `gorm:"not null;default:now()" json:"updated_at"`
}

func (NotificationJob) TableName() string { return "notification_jobs" }
