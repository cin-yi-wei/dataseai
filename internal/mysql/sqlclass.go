package mysql

import (
	"errors"
	"regexp"
	"strings"
)

type Op string

const (
	OpSelect    Op = "SELECT"
	OpInsert    Op = "INSERT"
	OpUpdate    Op = "UPDATE"
	OpDelete    Op = "DELETE"
	OpTruncate  Op = "TRUNCATE"
	OpDDL       Op = "DDL"       // ALTER / RENAME on existing table
	OpForbidden Op = "FORBIDDEN" // CREATE / DROP / GRANT / etc — never allowed
	OpReadMeta  Op = "READMETA"  // SHOW / DESCRIBE / DESC / EXPLAIN
	OpUnknown   Op = "UNKNOWN"
)

type Classified struct {
	Op    Op
	DB    string
	Table string
	Multi bool
}

// stripComments removes /* ... */ block comments and `-- ...` line comments.
// Keeps content inside string literals and backtick identifiers intact.
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
		case c == '\'' || c == '"':
			b.WriteByte(c)
			quote := c
			i++
			for i < len(s) {
				if s[i] == '\\' && i+1 < len(s) {
					b.WriteByte(s[i])
					b.WriteByte(s[i+1])
					i += 2
					continue
				}
				b.WriteByte(s[i])
				if s[i] == quote {
					i++
					break
				}
				i++
			}
		case c == '`':
			b.WriteByte(c)
			i++
			for i < len(s) {
				if s[i] == '`' && i+1 < len(s) && s[i+1] == '`' {
					b.WriteByte('`')
					b.WriteByte('`')
					i += 2
					continue
				}
				b.WriteByte(s[i])
				if s[i] == '`' {
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

// splitTopLevel breaks a comment-stripped SQL string at top-level semicolons
// (i.e. semicolons not inside quotes or backticks). Returns trimmed non-empty parts.
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
				if s[i] == '\\' && i+1 < len(s) {
					cur.WriteByte(s[i])
					cur.WriteByte(s[i+1])
					i += 2
					continue
				}
				cur.WriteByte(s[i])
				if s[i] == quote {
					i++
					break
				}
				i++
			}
		case '`':
			cur.WriteByte(c)
			i++
			for i < len(s) {
				if s[i] == '`' && i+1 < len(s) && s[i+1] == '`' {
					cur.WriteByte('`')
					cur.WriteByte('`')
					i += 2
					continue
				}
				cur.WriteByte(s[i])
				if s[i] == '`' {
					i++
					break
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

var identRE = regexp.MustCompile("`(?:[^`]|``)*`|[A-Za-z_][A-Za-z0-9_$]*")

func firstTableRef(s string) (string, string) {
	s = strings.TrimSpace(s)
	m := identRE.FindStringIndex(s)
	if m == nil {
		return "", ""
	}
	first := unquote(s[m[0]:m[1]])
	rest := strings.TrimLeft(s[m[1]:], " \t\r\n")
	if strings.HasPrefix(rest, ".") {
		rest = strings.TrimLeft(rest[1:], " \t\r\n")
		m2 := identRE.FindStringIndex(rest)
		if m2 != nil {
			return first, unquote(rest[m2[0]:m2[1]])
		}
	}
	return "", first
}

func unquote(s string) string {
	if len(s) >= 2 && s[0] == '`' && s[len(s)-1] == '`' {
		inner := s[1 : len(s)-1]
		return strings.ReplaceAll(inner, "``", "`")
	}
	return s
}

var (
	reInsert   = regexp.MustCompile(`(?is)^INSERT\s+(?:LOW_PRIORITY\s+|DELAYED\s+|HIGH_PRIORITY\s+|IGNORE\s+)*INTO\s+(.+)$`)
	reUpdate   = regexp.MustCompile(`(?is)^UPDATE\s+(?:LOW_PRIORITY\s+|IGNORE\s+)*(.+)$`)
	reDelete   = regexp.MustCompile(`(?is)^DELETE\s+(?:LOW_PRIORITY\s+|QUICK\s+|IGNORE\s+)*FROM\s+(.+)$`)
	reTruncate = regexp.MustCompile(`(?is)^TRUNCATE\s+(?:TABLE\s+)?(.+)$`)
	reAlter    = regexp.MustCompile(`(?is)^ALTER\s+TABLE\s+(.+)$`)
	reRename   = regexp.MustCompile(`(?is)^RENAME\s+TABLE\s+(.+)$`)
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

func ClassifySQL(sql string) (Classified, error) {
	cleaned := strings.TrimSpace(stripComments(sql))
	if cleaned == "" {
		return Classified{Op: OpUnknown}, errors.New("empty sql")
	}
	parts := splitTopLevel(cleaned)
	if len(parts) == 0 {
		return Classified{Op: OpUnknown}, errors.New("empty sql")
	}
	multi := len(parts) > 1
	head := parts[0]
	verb := firstVerb(head)

	var c Classified
	c.Multi = multi
	switch verb {
	case "SELECT":
		c.Op = OpSelect
		if m := reFromTbl.FindStringSubmatch(head); m != nil {
			c.DB, c.Table = firstTableRef(m[1])
		}
	case "INSERT":
		if m := reInsert.FindStringSubmatch(head); m != nil {
			c.Op = OpInsert
			c.DB, c.Table = firstTableRef(m[1])
		} else {
			c.Op = OpUnknown
		}
	case "UPDATE":
		if m := reUpdate.FindStringSubmatch(head); m != nil {
			c.Op = OpUpdate
			c.DB, c.Table = firstTableRef(m[1])
		} else {
			c.Op = OpUnknown
		}
	case "DELETE":
		if m := reDelete.FindStringSubmatch(head); m != nil {
			c.Op = OpDelete
			c.DB, c.Table = firstTableRef(m[1])
		} else {
			c.Op = OpUnknown
		}
	case "TRUNCATE":
		if m := reTruncate.FindStringSubmatch(head); m != nil {
			c.Op = OpTruncate
			c.DB, c.Table = firstTableRef(m[1])
		} else {
			c.Op = OpUnknown
		}
	case "ALTER":
		if m := reAlter.FindStringSubmatch(head); m != nil {
			c.Op = OpDDL
			c.DB, c.Table = firstTableRef(m[1])
		} else {
			c.Op = OpUnknown
		}
	case "RENAME":
		if m := reRename.FindStringSubmatch(head); m != nil {
			c.Op = OpDDL
			c.DB, c.Table = firstTableRef(m[1])
		} else {
			c.Op = OpUnknown
		}
	case "CREATE", "DROP", "GRANT", "REVOKE", "REPLACE", "FLUSH", "RESET":
		c.Op = OpForbidden
	case "SHOW", "DESCRIBE", "DESC", "EXPLAIN":
		c.Op = OpReadMeta
	default:
		c.Op = OpUnknown
	}
	return c, nil
}
