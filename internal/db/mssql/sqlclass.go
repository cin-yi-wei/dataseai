package mssql

import (
	"errors"
	"regexp"
	"strings"

	"github.com/conray/dataseai/internal/db"
)

// stripComments removes /* ... */ block comments and `-- ...` line comments
// while preserving content inside string literals and bracket/double-quoted
// identifiers.
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
					// T-SQL escapes single quotes by doubling them.
					if i+1 < len(s) && s[i+1] == '\'' {
						i++
						b.WriteByte(s[i])
					}
					i++
					break
				}
				i++
			}
		case c == '[':
			// bracket-quoted identifier
			b.WriteByte(c)
			i++
			for i < len(s) {
				b.WriteByte(s[i])
				if s[i] == ']' {
					if i+1 < len(s) && s[i+1] == ']' {
						i++
						b.WriteByte(s[i]) // escaped ]]
					} else {
						i++
						break
					}
				}
				i++
			}
		case c == '"':
			b.WriteByte(c)
			i++
			for i < len(s) {
				b.WriteByte(s[i])
				if s[i] == '"' {
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

// splitTopLevel splits at top-level semicolons (not inside quotes/brackets).
func splitTopLevel(s string) []string {
	var parts []string
	var cur strings.Builder
	i := 0
	for i < len(s) {
		c := s[i]
		switch c {
		case '\'', '"':
			cur.WriteByte(c)
			quote := c
			i++
			for i < len(s) {
				cur.WriteByte(s[i])
				if s[i] == quote {
					i++
					break
				}
				i++
			}
		case '[':
			cur.WriteByte(c)
			i++
			for i < len(s) {
				cur.WriteByte(s[i])
				if s[i] == ']' {
					if i+1 < len(s) && s[i+1] == ']' {
						i++
						cur.WriteByte(s[i])
					} else {
						i++
						break
					}
				}
				i++
			}
		case ';':
			t := strings.TrimSpace(cur.String())
			if t != "" {
				parts = append(parts, t)
			}
			cur.Reset()
			i++
		default:
			cur.WriteByte(c)
			i++
		}
	}
	t := strings.TrimSpace(cur.String())
	if t != "" {
		parts = append(parts, t)
	}
	return parts
}

// identRE matches a T-SQL identifier: bracket-quoted, double-quoted, or bare.
var identRE = regexp.MustCompile(`\[(?:[^\]]|\]\])*\]|"(?:[^"]|"")*"|[A-Za-z_#][A-Za-z0-9_#$@]*`)

func firstTableRef(s string) (string, string) {
	s = strings.TrimSpace(s)
	m := identRE.FindStringIndex(s)
	if m == nil {
		return "", ""
	}
	first := unquoteIdent(s[m[0]:m[1]])
	rest := strings.TrimLeft(s[m[1]:], " \t\r\n")
	if strings.HasPrefix(rest, ".") {
		rest = strings.TrimLeft(rest[1:], " \t\r\n")
		m2 := identRE.FindStringIndex(rest)
		if m2 != nil {
			return first, unquoteIdent(rest[m2[0]:m2[1]])
		}
	}
	return "", first
}

func unquoteIdent(s string) string {
	if len(s) >= 2 {
		if s[0] == '[' && s[len(s)-1] == ']' {
			return strings.ReplaceAll(s[1:len(s)-1], "]]", "]")
		}
		if s[0] == '"' && s[len(s)-1] == '"' {
			return strings.ReplaceAll(s[1:len(s)-1], `""`, `"`)
		}
	}
	return s
}

var (
	reInsert   = regexp.MustCompile(`(?is)^INSERT\s+INTO\s+(.+)$`)
	reUpdate   = regexp.MustCompile(`(?is)^UPDATE\s+(.+)$`)
	reDelete   = regexp.MustCompile(`(?is)^DELETE\s+(?:FROM\s+)?(.+)$`)
	reTruncate = regexp.MustCompile(`(?is)^TRUNCATE\s+(?:TABLE\s+)?(.+)$`)
	reAlter    = regexp.MustCompile(`(?is)^ALTER\s+TABLE\s+(.+)$`)
	reFromTbl  = regexp.MustCompile(`(?is)\bFROM\s+(.+)$`)
)

func firstVerb(stmt string) string {
	i := 0
	for i < len(stmt) && (stmt[i] == ' ' || stmt[i] == '\t' || stmt[i] == '\n' || stmt[i] == '\r') {
		i++
	}
	j := i
	for j < len(stmt) && ((stmt[j] >= 'A' && stmt[j] <= 'Z') || (stmt[j] >= 'a' && stmt[j] <= 'z')) {
		j++
	}
	return strings.ToUpper(stmt[i:j])
}

// ClassifySQL classifies a T-SQL statement, extracting op kind, schema/table,
// and multi-statement flag.
func (MSSQL) ClassifySQL(sql string) (db.Classified, error) {
	cleaned := strings.TrimSpace(stripComments(sql))
	if cleaned == "" {
		return db.Classified{Op: db.OpUnknown}, errors.New("empty sql")
	}
	parts := splitTopLevel(cleaned)
	if len(parts) == 0 {
		return db.Classified{Op: db.OpUnknown}, errors.New("empty sql")
	}
	multi := len(parts) > 1
	head := parts[0]
	verb := firstVerb(head)

	var c db.Classified
	c.Multi = multi
	switch verb {
	case "SELECT":
		c.Op = db.OpSelect
		if m := reFromTbl.FindStringSubmatch(head); m != nil {
			c.DB, c.Table = firstTableRef(m[1])
		}
	case "WITH":
		c.Op = db.OpSelect
	case "INSERT":
		if m := reInsert.FindStringSubmatch(head); m != nil {
			c.Op = db.OpInsert
			c.DB, c.Table = firstTableRef(m[1])
		} else {
			c.Op = db.OpUnknown
		}
	case "UPDATE":
		if m := reUpdate.FindStringSubmatch(head); m != nil {
			c.Op = db.OpUpdate
			c.DB, c.Table = firstTableRef(m[1])
		} else {
			c.Op = db.OpUnknown
		}
	case "DELETE":
		if m := reDelete.FindStringSubmatch(head); m != nil {
			c.Op = db.OpDelete
			c.DB, c.Table = firstTableRef(m[1])
		} else {
			c.Op = db.OpUnknown
		}
	case "TRUNCATE":
		if m := reTruncate.FindStringSubmatch(head); m != nil {
			c.Op = db.OpTruncate
			c.DB, c.Table = firstTableRef(m[1])
		} else {
			c.Op = db.OpUnknown
		}
	case "ALTER":
		if m := reAlter.FindStringSubmatch(head); m != nil {
			c.Op = db.OpDDL
			c.DB, c.Table = firstTableRef(m[1])
		} else {
			c.Op = db.OpUnknown
		}
	case "CREATE", "DROP", "GRANT", "REVOKE", "DENY":
		c.Op = db.OpForbidden
	case "EXEC", "EXECUTE", "SHOW", "EXPLAIN", "PRINT", "SET", "USE":
		c.Op = db.OpReadMeta
	default:
		c.Op = db.OpUnknown
	}
	return c, nil
}
