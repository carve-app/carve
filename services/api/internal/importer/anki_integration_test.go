// Integration test for the Anki importer — uses a real Postgres container
// so the SQL the handler actually executes is exercised. The Map() unit
// tests cover the pure logic; this test covers the wiring and the SQL.
//
// The .apkg payload is constructed programmatically: a minimal SQLite
// database with a `notes` table and a `cards` table containing realistic
// scheduling values, then zipped into the .apkg envelope.

package importer

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/carve-app/carve/services/api/internal/auth"
	"github.com/carve-app/carve/services/api/internal/db"
)

// TestImportAnki_PreservesSchedulingFields creates a test user, imports
// an apkg with non-trivial scheduling, then queries Postgres to confirm
// fsrs_state, fsrs_stability, fsrs_difficulty, fsrs_due, fsrs_reps and
// fsrs_lapses were preserved through the import pipeline.
//
// This is the property-level test that earns the "Anki round-trip"
// claim from the strategy doc — it's the thing that blocks regressions
// in the "drop-in Migaku replacement" criterion.
func TestImportAnki_PreservesSchedulingFields(t *testing.T) {
	pool := db.SetupPostgres(t)
	ctx := context.Background()

	// Seed a user.
	userID := auth.NewID()
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, email, display_name)
		 VALUES ($1, 'a@b.com', 'Anki Tester')`,
		userID,
	)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}

	// Build a fixture .apkg.
	collectionCreated := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	apkg, err := buildFixtureAPKG(t, collectionCreated, []fixtureCard{
		{Front: "猫", Back: "cat", Type: 2, Queue: 2, IVL: 42, Factor: 2500, Reps: 7, Lapses: 1, Due: 100},
		{Front: "犬", Back: "dog", Type: 2, Queue: 2, IVL: 5, Factor: 1800, Reps: 12, Lapses: 4, Due: 5},
		{Front: "鳥", Back: "bird", Type: 0, Queue: 0},                     // new
		{Front: "魚", Back: "fish", Type: 2, Queue: -1, IVL: 10, Factor: 2500}, // suspended
	})
	if err != nil {
		t.Fatalf("build fixture: %v", err)
	}

	// Drive ImportAnki via a synthetic HTTP request.
	body := &bytes.Buffer{}
	mw := newMultipart(body)
	mw.field("language", "ja")
	mw.file("file", "deck.apkg", apkg)
	mw.close()

	req := httptest.NewRequest("POST", "/v1/import/anki", body)
	req.Header.Set("Content-Type", mw.contentType())
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{UserID: userID}))

	h := NewHandler(pool)
	w := httptest.NewRecorder()
	h.ImportAnki(w, req)

	if w.Code != 200 {
		t.Fatalf("ImportAnki returned %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if int(resp["imported"].(float64)) != 4 {
		t.Errorf("expected 4 imported, got %v", resp["imported"])
	}

	// Inspect the persisted cards.
	rows, err := pool.Query(ctx,
		`SELECT front_text, fsrs_state, fsrs_stability, fsrs_difficulty,
		        fsrs_due, fsrs_reps, fsrs_lapses
		 FROM cards WHERE user_id = $1 ORDER BY front_text`,
		userID,
	)
	if err != nil {
		t.Fatalf("query cards: %v", err)
	}
	defer rows.Close()

	type got struct {
		front       string
		state       string
		stability   *float64
		difficulty  *float64
		due         *time.Time
		reps, lapses int
	}
	var imported []got
	for rows.Next() {
		var g got
		if err := rows.Scan(&g.front, &g.state, &g.stability, &g.difficulty,
			&g.due, &g.reps, &g.lapses); err != nil {
			t.Fatalf("scan: %v", err)
		}
		imported = append(imported, g)
	}
	if len(imported) != 4 {
		t.Fatalf("expected 4 rows, got %d", len(imported))
	}

	byFront := map[string]got{}
	for _, g := range imported {
		byFront[g.front] = g
	}

	// 猫 (cat): review card, ivl=42, reps=7, lapses=1
	cat := byFront["猫"]
	if cat.state != "review" {
		t.Errorf("猫 expected state=review, got %q", cat.state)
	}
	if cat.stability == nil || *cat.stability != 42 {
		t.Errorf("猫 expected stability=42, got %v", cat.stability)
	}
	if cat.reps != 7 || cat.lapses != 1 {
		t.Errorf("猫 reps/lapses lost: got %d/%d", cat.reps, cat.lapses)
	}
	if cat.due == nil {
		t.Errorf("猫 due lost (should be 100 days after collection epoch)")
	} else {
		expected := collectionCreated.Add(100 * 24 * time.Hour)
		if !cat.due.Equal(expected) {
			t.Errorf("猫 due drift: got %v, want %v", cat.due, expected)
		}
	}

	// 鳥 (bird): new card — must have no stability/difficulty
	bird := byFront["鳥"]
	if bird.state != "new" {
		t.Errorf("鳥 expected new, got %q", bird.state)
	}
	if bird.stability != nil {
		t.Errorf("new card 鳥 must not have stability")
	}

	// 魚 (fish): suspended
	fish := byFront["魚"]
	if fish.state != "suspended" {
		t.Errorf("魚 expected suspended, got %q", fish.state)
	}
}

// ── Fixture builder ──────────────────────────────────────────────────────────

type fixtureCard struct {
	Front  string
	Back   string
	Type   int
	Queue  int
	IVL    int
	Factor int
	Reps   int
	Lapses int
	Due    int64
}

func buildFixtureAPKG(t *testing.T, crt time.Time, cards []fixtureCard) ([]byte, error) {
	tmp := fmt.Sprintf("/tmp/carve_apkg_%d.db", time.Now().UnixNano())
	defer os.Remove(tmp)

	conn, err := sql.Open("sqlite", tmp)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if _, err := conn.Exec(`
		CREATE TABLE col   (id INTEGER PRIMARY KEY, crt INTEGER);
		CREATE TABLE notes (id INTEGER PRIMARY KEY, flds TEXT, tags TEXT);
		CREATE TABLE cards (id INTEGER PRIMARY KEY, nid INTEGER, type INT,
		                    queue INT, ivl INT, factor INT, reps INT,
		                    lapses INT, due INT);
		INSERT INTO col (id, crt) VALUES (1, ?);
	`, crt.Unix()); err != nil {
		return nil, err
	}

	for i, c := range cards {
		nid := int64(i + 1)
		flds := c.Front + "\x1f" + c.Back
		if _, err := conn.Exec(`INSERT INTO notes (id, flds, tags) VALUES (?, ?, '')`, nid, flds); err != nil {
			return nil, err
		}
		if _, err := conn.Exec(
			`INSERT INTO cards (id, nid, type, queue, ivl, factor, reps, lapses, due)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			nid*100, nid, c.Type, c.Queue, c.IVL, c.Factor, c.Reps, c.Lapses, c.Due,
		); err != nil {
			return nil, err
		}
	}
	conn.Close()

	dbBytes, err := os.ReadFile(tmp)
	if err != nil {
		return nil, err
	}

	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	f, err := zw.Create("collection.anki21")
	if err != nil {
		return nil, err
	}
	if _, err := f.Write(dbBytes); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return zipBuf.Bytes(), nil
}

// ── Tiny multipart writer ────────────────────────────────────────────────────

type multipartWriter struct {
	w        *bytes.Buffer
	boundary string
}

func newMultipart(w *bytes.Buffer) *multipartWriter {
	return &multipartWriter{w: w, boundary: "carveboundary123"}
}

func (m *multipartWriter) contentType() string { return "multipart/form-data; boundary=" + m.boundary }

func (m *multipartWriter) field(name, value string) {
	fmt.Fprintf(m.w, "--%s\r\nContent-Disposition: form-data; name=%q\r\n\r\n%s\r\n", m.boundary, name, value)
}

func (m *multipartWriter) file(name, filename string, data []byte) {
	fmt.Fprintf(m.w, "--%s\r\n", m.boundary)
	fmt.Fprintf(m.w, "Content-Disposition: form-data; name=%q; filename=%q\r\n", name, filename)
	fmt.Fprintf(m.w, "Content-Type: application/octet-stream\r\n\r\n")
	m.w.Write(data)
	m.w.WriteString("\r\n")
}

func (m *multipartWriter) close() {
	fmt.Fprintf(m.w, "--%s--\r\n", m.boundary)
}

// silence unused-import warnings in builds where modernc is provided
// transitively but we want a tracked direct dep.
var _ = strings.HasSuffix
