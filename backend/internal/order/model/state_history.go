package model

import (
	"time"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/order/state"
)

type OrderStateHistory struct {
	ID         uuid.UUID     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	OrderID    uuid.UUID     `gorm:"type:uuid;not null;index"                       json:"order_id"`
	FromStatus *state.Status `gorm:"size:50"                                        json:"from_status,omitempty"`
	ToStatus   state.Status  `gorm:"size:50;not null"                               json:"to_status"`
	ChangedBy  *uuid.UUID    `gorm:"type:uuid"                                      json:"changed_by,omitempty"`
	ChangedAt  time.Time     `gorm:"not null;default:now()"                         json:"changed_at"`
	Note       *string       `                                                      json:"note,omitempty"`
}

func (OrderStateHistory) TableName() string { return "order_state_history" }
