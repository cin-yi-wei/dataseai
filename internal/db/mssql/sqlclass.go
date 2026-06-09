package mssql

import (
	"strings"

	"github.com/conray/dataseai/internal/db"
)

// ClassifySQL parses a T-SQL statement to determine its operation kind and
// whether it touches multiple statements (semicolon-separated).
func (MSSQL) ClassifySQL(s string) (db.Classified, error) {
	stripped := stripComments(s)
	word := firstWord(stripped)

	var op db.Op
	switch strings.ToUpper(word) {
	case "SELECT", "WITH", "VALUES", "TABLE":
		op = db.OpSelect
	case "SHOW", "EXEC", "EXECUTE", "PRINT", "SET", "USE":
		op = db.OpReadMeta
	case "INSERT":
		op = db.OpInsert
	case "UPDATE":
		op = db.OpUpdate
	case "DELETE":
		op = db.OpDelete
	case "TRUNCATE":
		op = db.OpTruncate
	case "CREATE", "ALTER", "DROP", "RENAME", "INDEX":
		op = db.OpDDL
	case "GRANT", "REVOKE", "DENY":
		op = db.OpForbidden
	default:
		op = db.OpUnknown
	}

	multi := strings.Count(stripped, ";") > 1 ||
		(strings.Count(stripped, ";") == 1 && !strings.HasSuffix(strings.TrimSpace(stripped), ";"))

	return db.Classified{Op: op, Multi: multi}, nil
}

func firstWord(s string) string {
	s = strings.TrimLeft(s, " \t\r\n")
	end := 0
	for end < len(s) {
		c := s[end]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' {
			end++
			continue
		}
		break
	}
	return s[:end]
}

func stripComments(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == '-' && i+1 < len(s) && s[i+1] == '-':
			j := i + 2
			for j < len(s) && s[j] != '\n' {
				j++
			}
			i = j
		case c == '/' && i+1 < len(s) && s[i+1] == '*':
			j := i + 2
			for j+1 < len(s) && !(s[j] == '*' && s[j+1] == '/') {
				j++
			}
			i = j + 2
			if i > len(s) {
				i = len(s)
			}
		case c == '\'':
			b.WriteByte(c)
			i++
			for i < len(s) {
				b.WriteByte(s[i])
				if s[i] == '\'' {
					i++
					break
				}
				i++
			}
		case c == '[':
			// bracket-quoted identifier — pass through
			b.WriteByte(c)
			i++
			for i < len(s) {
				b.WriteByte(s[i])
				if s[i] == ']' {
					i++
					break
				}
				i++
			}
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String()
}
