// Package db connects to PostgreSQL and provides small helpers: All and One
// run a query and copy the rows into Go structs, Insert returns the new id.
package db

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib" // Postgres driver, registers itself as "pgx"
)

//go:embed schema.sql
var schema string

// TimeLayout is how times are shown and typed in forms: the same format an
// <input type="datetime-local"> uses ("2026-10-20T10:00"), in local time.
const TimeLayout = "2006-01-02T15:04"

// Querier is anything we can run SQL on: the database or a transaction.
// Functions accept a Querier so they work both inside and outside one.
type Querier interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// DB and Tx wrap database/sql so our queries can use "?" placeholders
// (easy to read and build up) while Postgres gets the "$1, $2" it needs.
type DB struct{ *sql.DB }
type Tx struct{ *sql.Tx }

func (d *DB) Exec(q string, args ...any) (sql.Result, error) { return d.DB.Exec(rebind(q), args...) }
func (d *DB) Query(q string, args ...any) (*sql.Rows, error) { return d.DB.Query(rebind(q), args...) }
func (d *DB) QueryRow(q string, args ...any) *sql.Row        { return d.DB.QueryRow(rebind(q), args...) }
func (t *Tx) Exec(q string, args ...any) (sql.Result, error) { return t.Tx.Exec(rebind(q), args...) }
func (t *Tx) Query(q string, args ...any) (*sql.Rows, error) { return t.Tx.Query(rebind(q), args...) }
func (t *Tx) QueryRow(q string, args ...any) *sql.Row        { return t.Tx.QueryRow(rebind(q), args...) }

func (d *DB) Begin() (*Tx, error) {
	tx, err := d.DB.Begin()
	return &Tx{tx}, err
}

// Open connects using a URL like postgres://user:pass@host:5432/dbname and
// creates any missing tables.
func Open(url string) (*DB, error) {
	conn, err := sql.Open("pgx", url)
	if err != nil {
		return nil, err
	}
	// Free hosted databases allow only a few connections, so keep the pool small.
	conn.SetMaxOpenConns(10)
	conn.SetMaxIdleConns(5)
	conn.SetConnMaxIdleTime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := conn.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("cannot reach the database (check DATABASE_URL): %w", err)
	}
	if _, err := conn.ExecContext(ctx, schema); err != nil {
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &DB{conn}, nil
}

// rebind turns "?" placeholders into Postgres's "$1", "$2"... in order.
// Question marks inside 'quoted strings' are left alone.
func rebind(query string) string {
	if !strings.Contains(query, "?") {
		return query
	}
	var b strings.Builder
	n, inQuote := 0, false
	for _, r := range query {
		switch {
		case r == '\'':
			inQuote = !inQuote
			b.WriteRune(r)
		case r == '?' && !inQuote:
			n++
			b.WriteString("$" + strconv.Itoa(n))
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Insert runs an INSERT and returns the new row's id.
func Insert(q Querier, query string, args ...any) (int64, error) {
	var id int64
	err := q.QueryRow(query+" RETURNING id", args...).Scan(&id)
	return id, err
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

// fieldsByTag maps "column_name" -> struct field index.
func fieldsByTag(t reflect.Type) map[string][]int {
	out := map[string][]int{}
	for i := 0; i < t.NumField(); i++ {
		if tag := t.Field(i).Tag.Get("db"); tag != "" {
			out[tag] = t.Field(i).Index
		}
	}
	return out
}

// setField converts a value from the driver (int64, float64, bool, string,
// []byte, time.Time or nil) into whatever type the struct field has.
// Times are stored in string fields as local "2026-10-20T10:00", which is
// what the templates and date inputs expect.
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
		case time.Time:
			field.SetString(r.In(time.Local).Format(TimeLayout))
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
		r, ok := raw.(bool)
		if !ok {
			return errors.New("expected true or false")
		}
		field.SetBool(r)
	default:
		return fmt.Errorf("unsupported field type %s", field.Type())
	}
	return nil
}

// IsUniqueViolation reports whether err came from a UNIQUE constraint
// (Postgres error code 23505).
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
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

// LocalTime turns a form value like "2026-10-20T10:00" (local time) into a
// time.Time for the database. Empty means NULL.
func LocalTime(s string) (any, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	for _, layout := range []string{TimeLayout, "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, nil
		}
	}
	return nil, fmt.Errorf("invalid date %q", s)
}
