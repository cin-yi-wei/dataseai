package pg

import (
	"errors"
	"regexp"
	"strings"

	"github.com/conray/dataseai/internal/db"
)

// stripComments removes /* ... */ block comments and `-- ...` line comments.
// Keeps content inside string literals and double-quoted identifiers intact.
// PostgreSQL uses '...' for string literals and "..." for delimited identifiers.
// There is no backtick quoting in PG.
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
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String()
}

// splitTopLevel breaks a comment-stripped SQL string at top-level semicolons
// (i.e. semicolons not inside quotes). Returns trimmed non-empty parts.
// PG only quotes with ' and "; no backticks.
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

// identRE matches a PostgreSQL identifier: either a double-quoted form
// (with "" as an embedded-quote escape) or a bare ASCII identifier.
var identRE = regexp.MustCompile(`"(?:[^"]|"")*"|[A-Za-z_][A-Za-z0-9_$]*`)

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
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		inner := s[1 : len(s)-1]
		return strings.ReplaceAll(inner, `""`, `"`)
	}
	return s
}

var (
	// PG variants of the DML/DDL prefix regexes. PG has no LOW_PRIORITY /
	// DELAYED / QUICK / IGNORE / HIGH_PRIORITY modifiers, but does accept
	// the ONLY keyword on UPDATE / DELETE / ALTER TABLE, and TRUNCATE may
	// omit the TABLE keyword. RETURNING suffixes don't affect op detection
	// and the regex stays anchored to ^VERB ... — the rest is captured wholesale.
	reInsert   = regexp.MustCompile(`(?is)^INSERT\s+INTO\s+(.+)$`)
	reUpdate   = regexp.MustCompile(`(?is)^UPDATE\s+(?:ONLY\s+)?(.+)$`)
	reDelete   = regexp.MustCompile(`(?is)^DELETE\s+(?:FROM\s+)?(?:ONLY\s+)?(.+)$`)
	reTruncate = regexp.MustCompile(`(?is)^TRUNCATE\s+(?:TABLE\s+)?(.+)$`)
	reAlter    = regexp.MustCompile(`(?is)^ALTER\s+TABLE\s+(?:ONLY\s+)?(.+)$`)
	// PG uses `ALTER TABLE x RENAME TO y`; the MySQL-style `RENAME TABLE`
	// is not valid PG. Keep the pattern for defensive classification so we
	// label it as DDL rather than Unknown if a user types it.
	reRename  = regexp.MustCompile(`(?is)^RENAME\s+TABLE\s+(.+)$`)
	reFromTbl = regexp.MustCompile(`(?is)\bFROM\s+(.+)$`)
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

// ClassifySQL classifies a PG SQL statement.
//
// Notable PG-specific behaviors:
//   - SELECT with a leading `WITH [RECURSIVE] ... AS (...)` CTE prelude
//     is classified as OpSelect with empty Table — extracting the
//     real FROM target reliably requires a parser. Policy layer must
//     accept Table=="" for SELECT.
//   - DELETE accepts `ONLY` prefix.
//   - INSERT/UPDATE/DELETE may carry a RETURNING suffix; the regex
//     captures it as part of the trailing block and firstTableRef
//     stops at the first identifier so it's harmless.
//   - Forbidden verbs include PG-only: CLUSTER, VACUUM, ANALYZE,
//     REINDEX, LISTEN, NOTIFY, UNLISTEN, COPY, SECURITY (e.g.
//     ALTER ... OWNER, SECURITY LABEL).
//   - SHOW is a valid PG read-meta verb (e.g. SHOW search_path).
func (PG) ClassifySQL(sql string) (db.Classified, error) {
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
		// WITH [RECURSIVE] cte AS (...) SELECT/INSERT/UPDATE/DELETE ...
		// Extracting the final operation's target reliably requires
		// matching parentheses through the CTE definitions. Classifier
		// records OpSelect with empty Table; the policy layer must
		// accept that.
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
	case "RENAME":
		if m := reRename.FindStringSubmatch(head); m != nil {
			c.Op = db.OpDDL
			c.DB, c.Table = firstTableRef(m[1])
		} else {
			c.Op = db.OpUnknown
		}
	case "CREATE", "DROP", "GRANT", "REVOKE",
		"CLUSTER", "VACUUM", "ANALYZE", "REINDEX",
		"LISTEN", "NOTIFY", "UNLISTEN", "COPY", "SECURITY":
		c.Op = db.OpForbidden
	case "SHOW", "EXPLAIN":
		c.Op = db.OpReadMeta
	default:
		c.Op = db.OpUnknown
	}
	return c, nil
}
