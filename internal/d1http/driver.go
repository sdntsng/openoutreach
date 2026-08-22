// Package d1http is a database/sql driver that executes SQLite statements
// on Cloudflare D1 via the Worker /internal/d1 proxy.
package d1http

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

func init() {
	sql.Register("d1http", &Driver{})
}

// FormatDSN builds a driver DSN from proxy origin and internal token.
func FormatDSN(proxyURL, token string) string {
	return strings.TrimRight(strings.TrimSpace(proxyURL), "/") + "|" + token
}

// Driver implements database/sql/driver.Driver.
type Driver struct{}

func (d *Driver) Open(name string) (driver.Conn, error) {
	proxy, token, ok := strings.Cut(name, "|")
	if !ok || strings.TrimSpace(proxy) == "" {
		return nil, fmt.Errorf("d1http: DSN must be proxyURL|token")
	}
	return &conn{
		proxy:  strings.TrimRight(strings.TrimSpace(proxy), "/"),
		token:  token,
		client: &http.Client{Timeout: 60 * time.Second},
	}, nil
}

type conn struct {
	proxy, token string
	client       *http.Client
	mu           sync.Mutex
	closed       bool
}

func (c *conn) Prepare(query string) (driver.Stmt, error) {
	return &stmt{c: c, query: query}, nil
}

func (c *conn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}

func (c *conn) Begin() (driver.Tx, error) {
	return nopTx{}, nil
}

type nopTx struct{}

func (nopTx) Commit() error   { return nil }
func (nopTx) Rollback() error { return nil }

func (c *conn) exec(query string, args []driver.NamedValue) (driver.Result, error) {
	params := namedToParams(args)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, driver.ErrBadConn
	}
	resp, err := c.roundTrip(context.Background(), proxyReq{SQL: query, Params: params, Mode: "exec"})
	if err != nil {
		return nil, err
	}
	return d1Result{lastID: resp.Meta.LastRowID, rows: resp.Meta.Changes}, nil
}

func (c *conn) query(query string, args []driver.NamedValue) (driver.Rows, error) {
	params := namedToParams(args)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, driver.ErrBadConn
	}
	resp, err := c.roundTrip(context.Background(), proxyReq{SQL: query, Params: params, Mode: "query"})
	if err != nil {
		return nil, err
	}
	return &rows{columns: resp.Columns, data: resp.Rows}, nil
}

type stmt struct {
	c     *conn
	query string
}

func (s *stmt) Close() error { return nil }

func (s *stmt) NumInput() int { return -1 }

func (s *stmt) Exec(args []driver.Value) (driver.Result, error) {
	return s.c.exec(s.query, valuesToNamed(args))
}

func (s *stmt) Query(args []driver.Value) (driver.Rows, error) {
	return s.c.query(s.query, valuesToNamed(args))
}

type proxyReq struct {
	SQL    string `json:"sql"`
	Params []any  `json:"params"`
	Mode   string `json:"mode"`
}

type proxyMeta struct {
	Changes   int64 `json:"changes"`
	LastRowID int64 `json:"last_row_id"`
}

type proxyResp struct {
	Columns []string  `json:"columns"`
	Rows    [][]any   `json:"rows"`
	Meta    proxyMeta `json:"meta"`
	Error   string    `json:"error"`
}

func (c *conn) roundTrip(ctx context.Context, req proxyReq) (*proxyResp, error) {
	const maxAttempts = 5
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * 250 * time.Millisecond):
			}
		}
		out, err := c.roundTripOnce(ctx, req)
		if err == nil {
			return out, nil
		}
		lastErr = err
		if !isRetryableD1HTTPError(err) {
			return nil, err
		}
	}
	return nil, lastErr
}

func isRetryableD1HTTPError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "http 502") ||
		strings.Contains(msg, "http 503") ||
		strings.Contains(msg, "http 504")
}

func (c *conn) roundTripOnce(ctx context.Context, req proxyReq) (*proxyResp, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.proxy+"/internal/d1", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		httpReq.Header.Set("X-Internal-Token", c.token)
	}
	res, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("d1http: %w", err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	var out proxyResp
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("d1http: decode %s: %w", truncate(raw, 200), err)
	}
	if out.Error != "" {
		return nil, errors.New(out.Error)
	}
	if res.StatusCode >= 400 {
		return nil, fmt.Errorf("d1http: HTTP %d %s", res.StatusCode, truncate(raw, 200))
	}
	return &out, nil
}

type d1Result struct {
	lastID int64
	rows   int64
}

func (r d1Result) LastInsertId() (int64, error) { return r.lastID, nil }
func (r d1Result) RowsAffected() (int64, error) { return r.rows, nil }

type rows struct {
	columns []string
	data    [][]any
	i       int
}

func (r *rows) Columns() []string { return r.columns }

func (r *rows) Close() error { return nil }

func (r *rows) Next(dest []driver.Value) error {
	if r.i >= len(r.data) {
		return io.EOF
	}
	row := r.data[r.i]
	r.i++
	for i := range dest {
		if i >= len(row) {
			dest[i] = nil
			continue
		}
		dest[i] = convertValue(row[i])
	}
	return nil
}

func convertValue(v any) driver.Value {
	switch t := v.(type) {
	case nil:
		return nil
	case bool, string, []byte:
		return t
	case float64:
		if t == float64(int64(t)) {
			return int64(t)
		}
		return t
	default:
		return fmt.Sprint(t)
	}
}

func namedToParams(args []driver.NamedValue) []any {
	out := make([]any, len(args))
	for i, a := range args {
		out[i] = a.Value
	}
	return out
}

func valuesToNamed(args []driver.Value) []driver.NamedValue {
	out := make([]driver.NamedValue, len(args))
	for i, v := range args {
		out[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return out
}

func truncate(b []byte, n int) string {
	s := string(b)
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}
