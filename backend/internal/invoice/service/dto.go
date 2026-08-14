package service

import (
	"github.com/rajaku-printing/backend/internal/invoice/model"
)

// FileHandle — everything handler needs to stream PDF.
type FileHandle struct {
	Invoice *model.Invoice
	AbsPath string
}
