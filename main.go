package main

import (
	"database/sql"
	"flag"
	"fmt"
	"html"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	_ "github.com/drummonds/go-postgres"
	"github.com/drummonds/lofigui"
)

var db *sql.DB

func initDB(dbPath string) (*sql.DB, error) {
	database, err := sql.Open("pglike", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	_, err = database.Exec(`CREATE TABLE IF NOT EXISTS todos (
		id SERIAL PRIMARY KEY,
		title VARCHAR(500) NOT NULL,
		completed BOOLEAN DEFAULT FALSE,
		created_at TIMESTAMP DEFAULT NOW()
	)`)
	if err != nil {
		database.Close()
		return nil, fmt.Errorf("creating table: %w", err)
	}
	return database, nil
}

func listTodos() {
	rows, err := db.Query("SELECT id, title, completed FROM todos ORDER BY id")
	if err != nil {
		lofigui.Printf("Error listing todos: %v", err)
		return
	}
	defer rows.Close()

	var todos []struct {
		ID        int64
		Title     string
		Completed int64
	}
	for rows.Next() {
		var t struct {
			ID        int64
			Title     string
			Completed int64
		}
		if err := rows.Scan(&t.ID, &t.Title, &t.Completed); err != nil {
			lofigui.Printf("Error scanning todo: %v", err)
			return
		}
		todos = append(todos, t)
	}

	if len(todos) == 0 {
		lofigui.Markdown("*No todos yet. Add one below!*")
		return
	}

	for _, t := range todos {
		checked := ""
		class := ""
		if t.Completed == 1 {
			checked = "checked"
			class = "completed"
		}
		lofigui.HTML(fmt.Sprintf(`<div class="box" style="display:flex; align-items:center; gap:0.75rem; padding:0.75rem;">
  <form action="/toggle" method="post" style="margin:0;">
    <input type="hidden" name="id" value="%d">
    <input type="checkbox" %s onchange="this.form.submit()" style="width:1.2em;height:1.2em;">
  </form>
  <span class="%s" style="flex:1;">%s</span>
  <form action="/delete" method="post" style="margin:0;">
    <input type="hidden" name="id" value="%d">
    <button class="button is-small is-danger is-outlined" type="submit">Delete</button>
  </form>
</div>`, t.ID, checked, class, html.EscapeString(t.Title), t.ID))
	}
}

func createTodo(title string) error {
	_, err := db.Exec("INSERT INTO todos (title) VALUES ($1)", title)
	return err
}

func toggleTodo(id int64) error {
	_, err := db.Exec("UPDATE todos SET completed = NOT completed WHERE id = $1", id)
	return err
}

func deleteTodo(id int64) error {
	_, err := db.Exec("DELETE FROM todos WHERE id = $1", id)
	return err
}

func handleIndex(ctrl *lofigui.Controller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		lofigui.Reset()
		listTodos()

		lofigui.HTML(`<hr>
<form action="/create" method="post" style="display:flex; gap:0.5rem;">
  <input class="input" type="text" name="title" placeholder="What needs to be done?" required>
  <button class="button is-primary" type="submit">Add</button>
</form>`)

		context := ctrl.StateDict(r)
		context["content"] = lofigui.Buffer()
		ctrl.RenderTemplate(w, context)
	}
}

func handleCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	r.ParseForm()
	title := r.FormValue("title")
	if title != "" {
		if err := createTodo(title); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func handleToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	r.ParseForm()
	id, err := strconv.ParseInt(r.FormValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := toggleTodo(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	r.ParseForm()
	id, err := strconv.ParseInt(r.FormValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := deleteTodo(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func main() {
	port := flag.Int("port", 9004, "HTTP server port")
	dbDir := flag.String("db-dir", ".", "directory for the database file")
	flag.Parse()

	if envPort := os.Getenv("PGLIKE_TODO_PORT"); envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil {
			*port = p
		}
	}
	if envDir := os.Getenv("PGLIKE_TODO_DB_DIR"); envDir != "" {
		*dbDir = envDir
	}

	dbPath := filepath.Join(*dbDir, "todos.db")
	var err error
	db, err = initDB(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	ctrl, err := lofigui.NewController(lofigui.ControllerConfig{
		Name:         "Todo List",
		TemplatePath: "templates/todo.html",
	})
	if err != nil {
		log.Fatalf("Failed to create controller: %v", err)
	}

	http.HandleFunc("/", handleIndex(ctrl))
	http.HandleFunc("/create", handleCreate)
	http.HandleFunc("/toggle", handleToggle)
	http.HandleFunc("/delete", handleDelete)
	http.HandleFunc("/favicon.ico", lofigui.ServeFavicon)

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("Starting todo server on http://localhost%s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
