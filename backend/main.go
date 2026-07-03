package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

type HealthResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

type Todo struct {
	Id        string `json:"id"`
	Task      string `json:"task"`
	Completed bool   `json:"completed"`
}

func generateRandomId() string {
	return uuid.New().String()
}

var db *sql.DB

const createTodosTableQuery = `
CREATE TABLE IF NOT EXISTS todos (
	id TEXT PRIMARY KEY,
	task TEXT NOT NULL,
	completed BOOLEAN NOT NULL DEFAULT FALSE
);`

func getDatabaseURL() string {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return "postgres://postgres:postgres@localhost:5432/todo_golang?sslmode=disable"
	}
	return databaseURL
}

func initDatabase() (*sql.DB, error) {
	database, err := sql.Open("postgres", getDatabaseURL())
	if err != nil {
		return nil, err
	}

	if err := database.Ping(); err != nil {
		database.Close()
		return nil, err
	}

	if _, err := database.Exec(createTodosTableQuery); err != nil {
		database.Close()
		return nil, err
	}

	return database, nil
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	health := HealthResponse{
		Status:  "OK",
		Message: "API health is running well.",
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(health)
}

func todoHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Println(r.Method)
	switch r.Method {
	case "GET":
		w.Header().Set("Content-Type", "application/json")
		rows, err := db.Query("SELECT id, task, completed FROM todos ORDER BY id")
		if err != nil {
			http.Error(w, "unable to fetch todos", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		todos := []Todo{}
		for rows.Next() {
			var todo Todo
			if err := rows.Scan(&todo.Id, &todo.Task, &todo.Completed); err != nil {
				http.Error(w, "unable to read todo", http.StatusInternalServerError)
				return
			}
			todos = append(todos, todo)
		}

		if err := rows.Err(); err != nil {
			http.Error(w, "unable to read todos", http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(todos)
	case "POST":
		var newTodo Todo

		err := json.NewDecoder(r.Body).Decode(&newTodo)
		if err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}

		if newTodo.Task == "" {
			http.Error(w, "task is required", http.StatusBadRequest)
			return
		}

		newTodo.Id = generateRandomId()

		err = db.QueryRow(
			"INSERT INTO todos (id, task, completed) VALUES ($1, $2, $3) RETURNING id, task, completed",
			newTodo.Id,
			newTodo.Task,
			newTodo.Completed,
		).Scan(&newTodo.Id, &newTodo.Task, &newTodo.Completed)
		if err != nil {
			http.Error(w, "unable to create todo", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(newTodo)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func todoByHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/todos/"):]

	if id == "" {
		http.Error(w, "todo ID is required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case "GET":
		var todo Todo
		err := db.QueryRow("SELECT id, task, completed FROM todos WHERE id = $1", id).
			Scan(&todo.Id, &todo.Task, &todo.Completed)
		if err == sql.ErrNoRows {
			http.Error(w, "Todo not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "unable to fetch todo", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(todo)

	case "PUT":
		var updatedTodo Todo
		if err := json.NewDecoder(r.Body).Decode(&updatedTodo); err != nil {
			http.Error(w, "Invalid JSON body", http.StatusBadRequest)
			return
		}

		if updatedTodo.Task == "" {
			http.Error(w, "task is required", http.StatusBadRequest)
			return
		}

		var todo Todo
		err := db.QueryRow(
			"UPDATE todos SET task = $1, completed = $2 WHERE id = $3 RETURNING id, task, completed",
			updatedTodo.Task,
			updatedTodo.Completed,
			id,
		).Scan(&todo.Id, &todo.Task, &todo.Completed)
		if err == sql.ErrNoRows {
			http.Error(w, "Todo not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "unable to update todo", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(todo)

	case "DELETE":
		result, err := db.Exec("DELETE FROM todos WHERE id = $1", id)
		if err != nil {
			http.Error(w, "unable to delete todo", http.StatusInternalServerError)
			return
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			http.Error(w, "unable to delete todo", http.StatusInternalServerError)
			return
		}
		if rowsAffected == 0 {
			http.Error(w, "Todo not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"message": "Todo with ID " + id + " deleted successfully"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func main() {
	var err error
	db, err = initDatabase()
	if err != nil {
		log.Fatalf("Error connecting to database: %v", err)
	}
	defer db.Close()

	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/todos", todoHandler)
	http.HandleFunc("/todos/", todoByHandler)
	fmt.Println("APP is running on port 3000")
	err = http.ListenAndServe(":3000", nil)
	if err != nil {
		fmt.Printf("Error starting server: %v\n", err)
	}
}
