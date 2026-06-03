package api

import (
	"net/http"

	"github.com/conray/mysqlweb/internal/auth"
)

func handleActiveQueries(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		queries := d.QueryRegistry.List(u.ID)
		out := make([]map[string]any, 0, len(queries))
		for _, q := range queries {
			out = append(out, map[string]any{
				"query_id":    q.QueryID,
				"conn_id":     q.ConnID,
				"sql_excerpt": q.SQLExcerpt,
				"started_at":  q.StartedAt,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"queries": out})
	}
}
