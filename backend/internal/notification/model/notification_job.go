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
	// JobCancelled — deliberately cancelled by a business event BEFORE it was
	// sent (currently: order soft-deleted by super admin, § super admin order
	// tools — see order/service.Service.cancelPendingNotifications). Distinct
	// from JobDead: dead means "retries exhausted, needs human", cancelled
	// means "no longer relevant, will never be needed". Never auto-claimed
	// (repository.ClaimBatch only selects pending/failed).
	JobCancelled JobStatus = "cancelled"
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

	Status        JobStatus  `gorm:"size:20;not null;default:pending" json:"status"`
	Attempts      int        `gorm:"not null;default:0"               json:"attempts"`
	MaxAttempts   int        `gorm:"not null;default:5"               json:"max_attempts"`
	NextAttemptAt time.Time  `gorm:"not null;default:now()"           json:"next_attempt_at"`
	LastError     *string    `gorm:"type:text"                        json:"last_error,omitempty"`
	SentAt        *time.Time `                                        json:"sent_at,omitempty"`

	// IsSensitive — job's `message` carries a secret (currently: WhatsApp OTP
	// codes, review finding #3) that must NOT be kept in plaintext at rest
	// once the job no longer needs it. Repository.MarkSent/MarkFailure redact
	// `message` inline the moment a sensitive job reaches a terminal status
	// (sent/dead); Service.RedactStaleSensitiveMessages is a periodic sweep
	// safety net for jobs that never reach one. Never exposed to the worker —
	// not part of the ClaimedJob wire contract.
	IsSensitive bool `gorm:"not null;default:false" json:"-"`

	OrderID *uuid.UUID `gorm:"type:uuid" json:"order_id,omitempty"`

	CreatedAt time.Time `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt time.Time `gorm:"not null;default:now()" json:"updated_at"`
}

func (NotificationJob) TableName() string { return "notification_jobs" }
