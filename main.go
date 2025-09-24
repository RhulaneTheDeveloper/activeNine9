package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/gorilla/mux"
	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email,omitempty"`
	Role     string `json:"role"` // buyer, entrepreneur, admin
}

var db *sql.DB

// helper to respond JSON
func respondJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

// connect to DB using environment variables
func initDB() (*sql.DB, error) {
	// example env vars:
	// DB_USER=appuser
	// DB_PASS=apppass123
	// DB_HOST=127.0.0.1
	// DB_PORT=3306
	// DB_NAME=street2screen
	user := os.Getenv("DB_USER")
	pass := os.Getenv("DB_PASS")
	host := os.Getenv("DB_HOST")
	port := os.Getenv("DB_PORT")
	name := os.Getenv("DB_NAME")

	if user == "" || pass == "" || host == "" || port == "" || name == "" {
		return nil, errors.New("DB_ environment variables not set (DB_USER,DB_PASS,DB_HOST,DB_PORT,DB_NAME)")
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&multiStatements=true", user, pass, host, port, name)
	return sql.Open("mysql", dsn)
}

// run schema (very basic migration)
func runMigrations(db *sql.DB) error {
	schema := `
CREATE TABLE IF NOT EXISTS users (
  id INT AUTO_INCREMENT PRIMARY KEY,
  username VARCHAR(100) NOT NULL UNIQUE,
  email VARCHAR(255),
  role ENUM('buyer','entrepreneur','admin') NOT NULL,
  password_hash VARCHAR(255) NOT NULL,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);`
	_, err := db.Exec(schema)
	return err
}

// Hash password with bcrypt
func hashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// Create default users if they don't exist
func ensureDefaultUsers(db *sql.DB) error {
	defaults := []struct {
		Username string
		Email    string
		Role     string
		Password string
	}{
		{"buyer", "buyer@example.com", "buyer", "buyerpass123"},
		{"entrepreneur", "entrepreneur@example.com", "entrepreneur", "entpass123"},
		{"admin", "admin@example.com", "admin", "adminpass123"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, u := range defaults {
		// check existence
		var id int
		err := db.QueryRowContext(ctx, "SELECT id FROM users WHERE username = ?", u.Username).Scan(&id)
		if err == sql.ErrNoRows {
			// insert
			pwHash, err := hashPassword(u.Password)
			if err != nil {
				return err
			}
			_, err = db.ExecContext(ctx, "INSERT INTO users (username, email, role, password_hash) VALUES (?, ?, ?, ?)",
				u.Username, u.Email, u.Role, pwHash)
			if err != nil {
				return err
			}
			log.Printf("Created default user %s\n", u.Username)
		} else if err != nil {
			return err
		} else {
			log.Printf("User %s already exists (id=%d)\n", u.Username, id)
		}
	}
	return nil
}

// Handlers: Create, Read, Update, Delete, List

// Create user (hashes password)
func createUserHandler(w http.ResponseWriter, r *http.Request) {
	type req struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Role     string `json:"role"`
		Password string `json:"password"`
	}
	var payload req
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if payload.Username == "" || payload.Password == "" || payload.Role == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "username, role and password are required"})
		return
	}
	if payload.Role != "buyer" && payload.Role != "entrepreneur" && payload.Role != "admin" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid role"})
		return
	}

	pwHash, err := hashPassword(payload.Password)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not hash password"})
		return
	}

	res, err := db.Exec("INSERT INTO users (username, email, role, password_hash) VALUES (?, ?, ?, ?)",
		payload.Username, payload.Email, payload.Role, pwHash)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	respondJSON(w, http.StatusCreated, map[string]interface{}{"id": id})
}

// Get user by id
func getUserHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	var u User
	err := db.QueryRow("SELECT id, username, email, role FROM users WHERE id = ?", id).Scan(&u.ID, &u.Username, &u.Email, &u.Role)
	if err == sql.ErrNoRows {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	} else if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, u)
}

// Update user (does not update password here; separate endpoint recommended)
func updateUserHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	type req struct {
		Email string `json:"email,omitempty"`
		Role  string `json:"role,omitempty"`
	}
	var payload req
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	// basic validation for role
	if payload.Role != "" && payload.Role != "buyer" && payload.Role != "entrepreneur" && payload.Role != "admin" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid role"})
		return
	}

	// build update dynamically (simple approach)
	query := "UPDATE users SET "
	args := []interface{}{}
	setParts := []string{}
	if payload.Email != "" {
		setParts = append(setParts, "email = ?")
		args = append(args, payload.Email)
	}
	if payload.Role != "" {
		setParts = append(setParts, "role = ?")
		args = append(args, payload.Role)
	}
	if len(setParts) == 0 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "no fields to update"})
		return
	}
	query += fmt.Sprintf("%s WHERE id = ?", join(setParts, ", "))
	args = append(args, id)

	_, err := db.Exec(query, args...)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// Delete user
func deleteUserHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	res, err := db.Exec("DELETE FROM users WHERE id = ?", id)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// List users (simple)
func listUsersHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT id, username, email, role FROM users")
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	users := []User{}
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.Role); err != nil {
			continue
		}
		users = append(users, u)
	}
	respondJSON(w, http.StatusOK, users)
}

// helper join (small replacement for strings.Join without importing extra package in this sample)
func join(parts []string, sep string) string {
	out := ""
	for i, p := range parts {
		out += p
		if i < len(parts)-1 {
			out += sep
		}
	}
	return out
}

func main() {
	var err error
	db, err = initDB()
	if err != nil {
		log.Fatalf("DB init error: %v. Make sure env vars are set.", err)
	}
	defer db.Close()

	// ensure DB alive
	if err = db.Ping(); err != nil {
		log.Fatalf("DB ping error: %v", err)
	}

	// migrations
	if err = runMigrations(db); err != nil {
		log.Fatalf("Migration error: %v", err)
	}

	// default users
	if err = ensureDefaultUsers(db); err != nil {
		log.Fatalf("default users error: %v", err)
	}

	r := mux.NewRouter()

	// user CRUD
	r.HandleFunc("/users", createUserHandler).Methods("POST")
	r.HandleFunc("/users", listUsersHandler).Methods("GET")
	r.HandleFunc("/users/{id:[0-9]+}", getUserHandler).Methods("GET")
	r.HandleFunc("/users/{id:[0-9]+}", updateUserHandler).Methods("PUT")
	r.HandleFunc("/users/{id:[0-9]+}", deleteUserHandler).Methods("DELETE")

	addr := ":8080"
	log.Printf("Server starting on %s\n", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
