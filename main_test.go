package main

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	_ "github.com/drummonds/go-postgres"
)

func setupTestDB(t *testing.T) {
	t.Helper()
	var err error
	db, err = sql.Open("pglike", ":memory:")
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS todos (
		id SERIAL PRIMARY KEY,
		title VARCHAR(500) NOT NULL,
		completed BOOLEAN DEFAULT FALSE,
		created_at TIMESTAMP DEFAULT NOW()
	)`)
	if err != nil {
		t.Fatalf("creating table: %v", err)
	}
	t.Cleanup(func() { db.Close() })
}

func TestInitDB(t *testing.T) {
	database, err := initDB(":memory:")
	if err != nil {
		t.Fatalf("initDB: %v", err)
	}
	defer database.Close()

	// Verify the table exists by inserting and querying
	_, err = database.Exec("INSERT INTO todos (title) VALUES ($1)", "test")
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	var count int
	err = database.QueryRow("SELECT count(*) FROM todos").Scan(&count)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

func TestCreateTodo(t *testing.T) {
	setupTestDB(t)

	err := createTodo("Buy groceries")
	if err != nil {
		t.Fatalf("createTodo: %v", err)
	}

	var title string
	var completed int64
	err = db.QueryRow("SELECT title, completed FROM todos WHERE id = 1").Scan(&title, &completed)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if title != "Buy groceries" {
		t.Errorf("title = %q, want %q", title, "Buy groceries")
	}
	if completed != 0 {
		t.Errorf("completed = %d, want 0", completed)
	}
}

func TestToggleTodo(t *testing.T) {
	setupTestDB(t)

	createTodo("Test toggle")

	// Toggle on
	err := toggleTodo(1)
	if err != nil {
		t.Fatalf("toggleTodo: %v", err)
	}

	var completed int64
	db.QueryRow("SELECT completed FROM todos WHERE id = 1").Scan(&completed)
	if completed != 1 {
		t.Errorf("after first toggle: completed = %d, want 1", completed)
	}

	// Toggle off
	err = toggleTodo(1)
	if err != nil {
		t.Fatalf("toggleTodo: %v", err)
	}

	db.QueryRow("SELECT completed FROM todos WHERE id = 1").Scan(&completed)
	if completed != 0 {
		t.Errorf("after second toggle: completed = %d, want 0", completed)
	}
}

func TestDeleteTodo(t *testing.T) {
	setupTestDB(t)

	createTodo("To delete")

	err := deleteTodo(1)
	if err != nil {
		t.Fatalf("deleteTodo: %v", err)
	}

	var count int
	db.QueryRow("SELECT count(*) FROM todos").Scan(&count)
	if count != 0 {
		t.Errorf("count after delete = %d, want 0", count)
	}
}

func TestMultipleTodos(t *testing.T) {
	setupTestDB(t)

	createTodo("First")
	createTodo("Second")
	createTodo("Third")

	var count int
	db.QueryRow("SELECT count(*) FROM todos").Scan(&count)
	if count != 3 {
		t.Errorf("count = %d, want 3", count)
	}

	// Delete the middle one
	deleteTodo(2)
	db.QueryRow("SELECT count(*) FROM todos").Scan(&count)
	if count != 2 {
		t.Errorf("count after delete = %d, want 2", count)
	}

	// Remaining should be First and Third
	rows, err := db.Query("SELECT title FROM todos ORDER BY id")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	var titles []string
	for rows.Next() {
		var title string
		rows.Scan(&title)
		titles = append(titles, title)
	}
	if len(titles) != 2 || titles[0] != "First" || titles[1] != "Third" {
		t.Errorf("titles = %v, want [First Third]", titles)
	}
}

func TestHandleCreateEndpoint(t *testing.T) {
	setupTestDB(t)

	form := url.Values{"title": {"Test from HTTP"}}
	req := httptest.NewRequest(http.MethodPost, "/create", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handleCreate(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}

	var title string
	db.QueryRow("SELECT title FROM todos WHERE id = 1").Scan(&title)
	if title != "Test from HTTP" {
		t.Errorf("title = %q, want %q", title, "Test from HTTP")
	}
}

func TestHandleToggleEndpoint(t *testing.T) {
	setupTestDB(t)
	createTodo("Toggle me")

	form := url.Values{"id": {"1"}}
	req := httptest.NewRequest(http.MethodPost, "/toggle", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handleToggle(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}

	var completed int64
	db.QueryRow("SELECT completed FROM todos WHERE id = 1").Scan(&completed)
	if completed != 1 {
		t.Errorf("completed = %d, want 1", completed)
	}
}

func TestHandleDeleteEndpoint(t *testing.T) {
	setupTestDB(t)
	createTodo("Delete me")

	form := url.Values{"id": {"1"}}
	req := httptest.NewRequest(http.MethodPost, "/delete", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handleDelete(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}

	var count int
	db.QueryRow("SELECT count(*) FROM todos").Scan(&count)
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

func TestHandleCreateGetRedirects(t *testing.T) {
	setupTestDB(t)

	req := httptest.NewRequest(http.MethodGet, "/create", nil)
	w := httptest.NewRecorder()

	handleCreate(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("GET /create status = %d, want %d", w.Code, http.StatusSeeOther)
	}
}

func TestHandleToggleInvalidID(t *testing.T) {
	setupTestDB(t)

	form := url.Values{"id": {"notanumber"}}
	req := httptest.NewRequest(http.MethodPost, "/toggle", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handleToggle(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}
