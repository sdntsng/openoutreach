package hosted

import (
	"database/sql"
	"strconv"
	"strings"

	"github.com/andersmyrmel/cold-cli/internal"
)

func rebind(db *sql.DB, query string) string {
	if internal.CurrentDialect() != internal.DialectPostgres || !strings.Contains(query, "?") {
		return query
	}
	var out strings.Builder
	n := 1
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			out.WriteByte('$')
			out.WriteString(strconv.Itoa(n))
			n++
			continue
		}
		out.WriteByte(query[i])
	}
	return out.String()
}

func exec(db *sql.DB, query string, args ...any) (sql.Result, error) {
	return db.Exec(rebind(db, query), args...)
}

func query(db *sql.DB, queryStr string, args ...any) (*sql.Rows, error) {
	return db.Query(rebind(db, queryStr), args...)
}

func queryRow(db *sql.DB, queryStr string, args ...any) *sql.Row {
	return db.QueryRow(rebind(db, queryStr), args...)
}
