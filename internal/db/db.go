// Package db opens the SQLite database and provides two small helpers,
// All and One, that run a query and copy the rows into Go structs.
package db

import (
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	_ "modernc.org/sqlite" // pure-Go SQLite driver, registers itself as "sqlite"
)

//go:embed schema.sql
var schema string

// Querier is anything we can run SQL on: the database itself or a transaction.
// Functions accept a Querier so they work both inside and outside a transaction.
type Querier interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// Open creates the database file (and folder) if needed and applies the schema.
func Open(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	// Pragmas in the DSN run on every new connection:
	//   foreign_keys  – enforce REFERENCES / ON DELETE CASCADE
	//   journal_mode  – WAL lets readers and a writer work at the same time
	//   busy_timeout  – wait up to 5s for a lock instead of failing immediately
	// _txlock=immediate takes the write lock when a transaction starts, which
	// avoids "database is locked" errors when two transactions race.
	dsn := "file:" + path +
		"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_txlock=immediate"
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if _, err := conn.Exec(schema); err != nil {
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return conn, nil
}

// All runs a query and returns every row as a T.
// Columns are matched to struct fields by their `db:"column_name"` tag.
func All[T any](q Querier, query string, args ...any) ([]T, error) {
	rows, err := q.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var out []T
	for rows.Next() {
		var item T
		if err := scanInto(rows, columns, &item); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// One runs a query and returns the first row, or (nil, nil) if there is none.
func One[T any](q Querier, query string, args ...any) (*T, error) {
	items, err := All[T](q, query, args...)
	if err != nil || len(items) == 0 {
		return nil, err
	}
	return &items[0], nil
}

// Count runs a "SELECT COUNT(*) ..." style query and returns the number.
func Count(q Querier, query string, args ...any) (int64, error) {
	var n int64
	err := q.QueryRow(query, args...).Scan(&n)
	return n, err
}

// scanInto reads the current row and stores each column in the struct field
// whose db tag matches. Unknown columns are ignored. SQL NULL becomes the
// field's zero value, or nil if the field is a pointer.
func scanInto(rows *sql.Rows, columns []string, dest any) error {
	values := make([]any, len(columns))
	ptrs := make([]any, len(columns))
	for i := range values {
		ptrs[i] = &values[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		return err
	}

	v := reflect.ValueOf(dest).Elem()
	fields := fieldsByTag(v.Type())
	for i, col := range columns {
		idx, ok := fields[col]
		if !ok {
			continue
		}
		if err := setField(v.FieldByIndex(idx), values[i]); err != nil {
			return fmt.Errorf("column %s: %w", col, err)
		}
	}
	return nil
}

// fieldsByTag maps "column_name" -> struct field index, including fields of
// embedded structs (so a TeamRow can embed Team and add extra columns).
func fieldsByTag(t reflect.Type) map[string][]int {
	out := map[string][]int{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			for col, idx := range fieldsByTag(f.Type) {
				out[col] = append([]int{i}, idx...)
			}
			continue
		}
		if tag := f.Tag.Get("db"); tag != "" {
			out[tag] = f.Index
		}
	}
	return out
}

// setField converts a raw SQLite value (int64, float64, string, []byte or nil)
// into whatever type the struct field has.
func setField(field reflect.Value, raw any) error {
	if raw == nil {
		field.Set(reflect.Zero(field.Type()))
		return nil
	}
	if field.Kind() == reflect.Pointer {
		ptr := reflect.New(field.Type().Elem())
		if err := setField(ptr.Elem(), raw); err != nil {
			return err
		}
		field.Set(ptr)
		return nil
	}

	switch field.Kind() {
	case reflect.String:
		switch r := raw.(type) {
		case string:
			field.SetString(r)
		case []byte:
			field.SetString(string(r))
		default:
			field.SetString(fmt.Sprint(r))
		}
	case reflect.Int, reflect.Int64:
		switch r := raw.(type) {
		case int64:
			field.SetInt(r)
		case float64:
			field.SetInt(int64(r))
		default:
			return errors.New("expected a number")
		}
	case reflect.Float64:
		switch r := raw.(type) {
		case int64:
			field.SetFloat(float64(r))
		case float64:
			field.SetFloat(r)
		default:
			return errors.New("expected a number")
		}
	case reflect.Bool:
		r, ok := raw.(int64)
		if !ok {
			return errors.New("expected 0 or 1")
		}
		field.SetBool(r != 0)
	default:
		return fmt.Errorf("unsupported field type %s", field.Type())
	}
	return nil
}

// IsUniqueViolation reports whether err came from a UNIQUE constraint.
func IsUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE")
}

// NullIfEmpty turns "" into SQL NULL so optional text columns stay NULL.
func NullIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

// NullIfZero turns 0 into SQL NULL, used for optional ids and numbers.
func NullIfZero(n int64) any {
	if n == 0 {
		return nil
	}
	return n
}
