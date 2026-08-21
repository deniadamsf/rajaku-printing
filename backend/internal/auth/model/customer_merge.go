package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// CustomerMergeSourcePhoneClaim is the only Source value that exists today —
// a guest identity absorbed during the authenticated phone-claim flow (§11
// satu pelanggan satu riwayat). A distinct constant (rather than a bare
// string literal scattered at call sites) leaves room for a future
// absorption path (mis. an admin-triggered manual merge) without having to
// guess the exact spelling used elsewhere.
const CustomerMergeSourcePhoneClaim = "phone_claim"

// CustomerMerge is the durable audit trail for absorbing a GUEST identity
// into a registered account (migration 000019) — the single source of truth
// answering "this order used to belong to whom, and when did it move" long
// after the zerolog line that used to be the only trace of it has rotated
// out of the logs.
//
// FromUserID is the absorbed guest row — kept, never deleted (a tombstone,
// see UserRepository.SetActive), so this FK always resolves. ToUserID is the
// account the guest's identity (and OrderIDs) moved into. OrderIDs is a
// point-in-time snapshot (Postgres UUID[]), deliberately NOT a child table
// with FKs — this is an audit record that must stay exactly as written, not
// a relational set maintained over time, and absorption is a rare event.
//
// Deliberately NOT mirrored as a `merged_into_user_id` column on `users` —
// this table is the only source of truth for "who got absorbed into whom";
// a second pointer elsewhere could drift from it.
type CustomerMerge struct {
	ID          uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	FromUserID  uuid.UUID      `gorm:"column:from_user_id;type:uuid;not null"          json:"from_user_id"`
	ToUserID    uuid.UUID      `gorm:"column:to_user_id;type:uuid;not null"            json:"to_user_id"`
	Phone       string         `gorm:"size:20;not null"                                json:"phone"`
	OrdersMoved int            `gorm:"column:orders_moved;not null"                    json:"orders_moved"`
	OrderIDs    pq.StringArray `gorm:"column:order_ids;type:uuid[];not null"           json:"order_ids"`
	Source      string         `gorm:"size:30;not null"                                json:"source"`
	MergedAt    time.Time      `gorm:"column:merged_at;not null;default:now()"         json:"merged_at"`
}

func (CustomerMerge) TableName() string { return "customer_merges" }

// NewCustomerMerge builds a CustomerMerge ready for
// CustomerMergeRepository.Create — the only place in this package that knows
// about pq.StringArray's on-the-wire `uuid[]` representation, so callers
// (service layer) only ever deal in plain []uuid.UUID. OrdersMoved is
// derived from len(orderIDs) rather than taken as a separate parameter, so
// the two can never disagree.
func NewCustomerMerge(fromUserID, toUserID uuid.UUID, phone string, orderIDs []uuid.UUID, source string) *CustomerMerge {
	ids := make(pq.StringArray, len(orderIDs))
	for i, id := range orderIDs {
		ids[i] = id.String()
	}
	return &CustomerMerge{
		FromUserID:  fromUserID,
		ToUserID:    toUserID,
		Phone:       phone,
		OrdersMoved: len(orderIDs),
		OrderIDs:    ids,
		Source:      source,
	}
}
