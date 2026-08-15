package admin

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"

	_ "github.com/jackc/pgx/v5/stdlib" // register pgx driver for sql.Open("pgx", ...)
	_ "modernc.org/sqlite"            // register sqlite driver for sqlx.Open("sqlite", ...)
)

// ---- test DB fixtures ----

// newSQLiteInMemoryDB returns a real in-memory SQLite *sqlx.DB so tests can
// actually execute the SQL produced by credentialFragmentsSelect() against
// the SQLite dialect. DriverName() reports "sqlite".
func newSQLiteInMemoryDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite in-memory: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// newPgxLabeledDB returns a *sqlx.DB whose DriverName() reports "pgx" without
// needing a running PostgreSQL server. credentialFragmentsSelect() only reads
// DriverName() for the dialect-selection tests, so no query is ever executed
// against this handle.
func newPgxLabeledDB(t *testing.T) *sqlx.DB {
	t.Helper()
	sqlx.BindDriver("pgx", sqlx.DOLLAR)
	rawDB, err := sql.Open("pgx", "postgres://localhost:5432/metapi_test?sslmode=disable")
	if err != nil {
		t.Fatalf("open pgx handle: %v", err)
	}
	t.Cleanup(func() { _ = rawDB.Close() })
	return sqlx.NewDb(rawDB, "pgx")
}

// newLabeledDB opens a real SQLite in-memory handle but relabels it with an
// arbitrary driver name. Used to prove that any non-"pgx" driver falls back to
// the SQLite SUBSTR-based fragment branch. No query is executed against it.
func newLabeledDB(t *testing.T, driverName string) *sqlx.DB {
	t.Helper()
	rawDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite handle: %v", err)
	}
	t.Cleanup(func() { _ = rawDB.Close() })
	return sqlx.NewDb(rawDB, driverName)
}

// ---- credentialFragmentsSelect: dialect string generation ----

func TestCredentialFragmentsSelectDialects(t *testing.T) {
	const column = "c.secret"
	const aliasBase = "cred"
	sqliteWant := "SUBSTR(c.secret, 1, 4) AS cred_prefix, SUBSTR(c.secret, -4) AS cred_suffix, LENGTH(c.secret) AS cred_len"
	pgxWant := "LEFT(c.secret, 4) AS cred_prefix, RIGHT(c.secret, 4) AS cred_suffix, LENGTH(c.secret) AS cred_len"

	var nilDB *sqlx.DB

	cases := []struct {
		name string
		db   *sqlx.DB
		want string
	}{
		{"sqlite_driver", newSQLiteInMemoryDB(t), sqliteWant},
		{"mysql_driver_falls_back_to_substr", newLabeledDB(t, "mysql"), sqliteWant},
		{"empty_driver_name_falls_back_to_substr", newLabeledDB(t, ""), sqliteWant},
		{"pgx_driver_uses_left_right", newPgxLabeledDB(t), pgxWant},
		{"nil_db_falls_back_to_substr", nilDB, sqliteWant},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := credentialFragmentsSelect(tc.db, column, aliasBase)
			if got != tc.want {
				t.Errorf("credentialFragmentsSelect(%s) =\n  %q\nwant\n  %q", tc.name, got, tc.want)
			}
		})
	}
}

// TestCredentialFragmentsSelectPostgresShape is a focused check that the pgx
// branch emits LEFT()/RIGHT()/LENGTH() and snake_case aliases, independent of
// the table-driven cases above.
func TestCredentialFragmentsSelectPostgresShape(t *testing.T) {
	db := newPgxLabeledDB(t)
	got := credentialFragmentsSelect(db, "at.token", "token")
	want := "LEFT(at.token, 4) AS token_prefix, RIGHT(at.token, 4) AS token_suffix, LENGTH(at.token) AS token_len"
	if got != want {
		t.Fatalf("credentialFragmentsSelect(pgx) =\n  %q\nwant\n  %q", got, want)
	}
}

// ---- credentialFragmentsSelect: SQLite execution ----

// TestCredentialFragmentsSelectSQLiteExecutable runs the exact SQL that
// credentialFragmentsSelect() generates for SQLite against a real in-memory
// SQLite DB and verifies the returned prefix/suffix/length fragments are the
// expected first-4 / last-4 / total-length of the secret, and that
// maskSecretFromFragments() rebuilt from those fragments matches maskSecret().
func TestCredentialFragmentsSelectSQLiteExecutable(t *testing.T) {
	db := newSQLiteInMemoryDB(t)

	secrets := []string{
		"abcdefghij1234567890", // long secret (>8 chars)
		"12345678",                // exactly 8 chars -> collapses to "****"
		"abc",                     // short secret (<8 chars) -> "****"
		"",                        // empty secret -> ""
	}

	for _, secret := range secrets {
		t.Run(fmt.Sprintf("len=%d", len(secret)), func(t *testing.T) {
			fragments := credentialFragmentsSelect(db, "secret", "cred")
			query := db.Rebind("SELECT " + fragments + " FROM (SELECT ? AS secret)")

			type credFragmentsRow struct {
				Prefix string `db:"cred_prefix"`
				Suffix string `db:"cred_suffix"`
				Len    int64  `db:"cred_len"`
			}
			var row credFragmentsRow
			if err := db.Get(&row, query, secret); err != nil {
				t.Fatalf("exec fragments query for secret %q: %v", secret, err)
			}

			wantPrefix := firstChars(secret, 4)
			wantSuffix := lastChars(secret, 4)
			if row.Prefix != wantPrefix {
				t.Errorf("prefix = %q, want %q", row.Prefix, wantPrefix)
			}
			if row.Suffix != wantSuffix {
				t.Errorf("suffix = %q, want %q", row.Suffix, wantSuffix)
			}
			if row.Len != int64(len(secret)) {
				t.Errorf("len = %d, want %d", row.Len, len(secret))
			}

			// The fragments must rebuild the exact maskSecret() output without
			// the plaintext secret ever being scanned into Go memory.
			gotMasked := maskSecretFromFragments(row.Prefix, row.Suffix, row.Len)
			wantMasked := maskSecret(secret)
			if gotMasked != wantMasked {
				t.Errorf("maskSecretFromFragments = %q, want %q (maskSecret)", gotMasked, wantMasked)
			}
		})
	}
}

// ---- maskSecretFromFragments: edge cases ----

func TestMaskSecretFromFragments(t *testing.T) {
	cases := []struct {
		name   string
		prefix any
		suffix any
		length int64
		want   string
	}{
		{"empty_secret_length_zero", "", "", 0, ""},
		{"nil_fragments_length_zero", nil, nil, 0, ""},
		{"negative_length", "abcd", "wxyz", -3, ""},
		{"length_one_collapses", "a", "a", 1, "****"},
		{"length_eight_collapses", "abcd", "wxyz", 8, "****"},
		{"length_just_below_threshold", "ab", "yz", 7, "****"},
		{"length_nine_reveals_fragments", "abcd", "wxyz", 9, "abcd****wxyz"},
		{"long_secret_string_fragments", "sk-a", "7890", 23, "sk-a****7890"},
		{"byte_slice_fragments", []byte("sk-a"), []byte("7890"), 23, "sk-a****7890"},
		{"nil_fragments_with_long_length", nil, nil, 23, "****"},
		{"mixed_string_and_byte_fragments", "sk-a", []byte("7890"), 23, "sk-a****7890"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := maskSecretFromFragments(tc.prefix, tc.suffix, tc.length)
			if got != tc.want {
				t.Errorf("maskSecretFromFragments(%v, %v, %d) = %q, want %q",
					tc.prefix, tc.suffix, tc.length, got, tc.want)
			}
		})
	}
}

// TestMaskSecretFromFragmentsMatchesMaskSecret is a property check: for a
// range of real secret strings, the masked output rebuilt from the
// first-4/last-4/length fragments must equal maskSecret() applied to the full
// plaintext. This is the core invariant the helper exists to guarantee.
func TestMaskSecretFromFragmentsMatchesMaskSecret(t *testing.T) {
	secrets := []string{
		"",
		"a",
		"ab",
		"abcdefgh",  // 8 -> "****"
		"abcdefghi", // 9 -> first4 + **** + last4
		"abcdefghij1234567890",
		"d8_a1b2c3d4e5f6a7b8c9d0",
		strings.Repeat("x", 64),
	}
	for _, secret := range secrets {
		t.Run(fmt.Sprintf("len=%d", len(secret)), func(t *testing.T) {
			prefix := firstChars(secret, 4)
			suffix := lastChars(secret, 4)
			got := maskSecretFromFragments(prefix, suffix, int64(len(secret)))
			want := maskSecret(secret)
			if got != want {
				t.Errorf("len=%d: maskSecretFromFragments = %q, want %q", len(secret), got, want)
			}
		})
	}
}

// ---- redactSearchAccountSecrets ----

func TestRedactSearchAccountSecrets(t *testing.T) {
	t.Run("nil_row_is_noop", func(t *testing.T) {
		redactSearchAccountSecrets(nil) // must not panic
	})

	t.Run("populated_row_masks_both_tokens", func(t *testing.T) {
		row := map[string]any{
			"accessTokenPrefix": "sk-a",
			"accessTokenSuffix": "7890",
			"accessTokenLen":    int64(23),
			"apiTokenPrefix":    "abcd",
			"apiTokenSuffix":    "wxyz",
			"apiTokenLen":       int64(12),
		}
		redactSearchAccountSecrets(row)

		if got, _ := row["accessTokenMasked"].(string); got != "sk-a****7890" {
			t.Errorf("accessTokenMasked = %q, want %q", got, "sk-a****7890")
		}
		if got, _ := row["apiTokenMasked"].(string); got != "abcd****wxyz" {
			t.Errorf("apiTokenMasked = %q, want %q", got, "abcd****wxyz")
		}
	})

	t.Run("zero_or_missing_length_leaves_unmasked", func(t *testing.T) {
		row := map[string]any{
			"accessTokenPrefix": "sk-a",
			"accessTokenSuffix": "7890",
			"accessTokenLen":    int64(0), // explicit zero -> skip
			"apiTokenPrefix":    "abcd",  // apiTokenLen missing entirely -> skip
			"apiTokenSuffix":    "wxyz",
		}
		redactSearchAccountSecrets(row)

		if _, ok := row["accessTokenMasked"]; ok {
			t.Errorf("accessTokenMasked should not be set when length == 0, got %v", row["accessTokenMasked"])
		}
		if _, ok := row["apiTokenMasked"]; ok {
			t.Errorf("apiTokenMasked should not be set when length is missing, got %v", row["apiTokenMasked"])
		}
	})
}

// ---- redactSearchTokenSecrets ----

func TestRedactSearchTokenSecrets(t *testing.T) {
	t.Run("nil_row_is_noop", func(t *testing.T) {
		redactSearchTokenSecrets(nil) // must not panic
	})

	t.Run("populated_row_masks_token", func(t *testing.T) {
		row := map[string]any{
			"tokenPrefix": "sk-a",
			"tokenSuffix": "7890",
			"tokenLen":    int64(23),
		}
		redactSearchTokenSecrets(row)

		if got, _ := row["tokenMasked"].(string); got != "sk-a****7890" {
			t.Errorf("tokenMasked = %q, want %q", got, "sk-a****7890")
		}
	})

	t.Run("zero_length_leaves_unmasked", func(t *testing.T) {
		row := map[string]any{
			"tokenPrefix": "sk-a",
			"tokenSuffix": "7890",
			"tokenLen":    int64(0),
		}
		redactSearchTokenSecrets(row)

		if _, ok := row["tokenMasked"]; ok {
			t.Errorf("tokenMasked should not be set when length == 0, got %v", row["tokenMasked"])
		}
	})
}

// ---- maskValue ----

func TestMaskValue(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  string
	}{
		{"empty_stays_empty", "", ""},
		{"single_char", "x", "****"},
		{"two_chars", "ab", "ab****"},
		{"exactly_ten_chars", "abcdefghij", "ab****"},
		{"eleven_chars_reveals_middle", "abcdefghijk", "abcdef****hijk"},
		{"cookie_short_value", "session=abcdefg", "ab****"},
		{"cookie_long_value", "k=secretvalue1234", "secret****1234"},
		{"cookie_empty_value", "k=", "****"},
		{"cookie_single_char_value", "k=v", "****"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := maskValue(tc.value); got != tc.want {
				t.Errorf("maskValue(%q) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

// ---- test helpers ----

// firstChars returns the first n characters of s, or all of s if shorter.
func firstChars(s string, n int) string {
	if len(s) < n {
		return s
	}
	return s[:n]
}

// lastChars returns the last n characters of s, or all of s if shorter.
func lastChars(s string, n int) string {
	if len(s) < n {
		return s
	}
	return s[len(s)-n:]
}
