package user

import (
	"database/sql"
	"errors"
	"net/mail"
	"strings"

	"github.com/mangahub/mangahub/pkg/models"
)

var (
	ErrUserExists   = errors.New("user already exists")
	ErrUserNotFound = errors.New("user not found")
	ErrInvalidEmail = errors.New("invalid email format")
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func validateEmail(email string) error {
	_, err := mail.ParseAddress(email)
	if err != nil {
		return ErrInvalidEmail
	}
	return nil
}

func (r *Repository) Create(id, username, email, passwordHash string) (*models.User, error) {
	username = strings.TrimSpace(username)
	email = strings.TrimSpace(strings.ToLower(email))

	if username == "" {
		return nil, errors.New("username is required")
	}
	if err := validateEmail(email); err != nil {
		return nil, err
	}

	var existing string
	err := r.db.QueryRow("SELECT id FROM users WHERE username = ? OR email = ?", username, email).Scan(&existing)
	if err == nil {
		return nil, ErrUserExists
	}
	if err != sql.ErrNoRows {
		return nil, err
	}

	_, err = r.db.Exec(
		"INSERT INTO users (id, username, email, password_hash) VALUES (?, ?, ?, ?)",
		id, username, email, passwordHash,
	)
	if err != nil {
		return nil, err
	}

	return r.GetByID(id)
}

func (r *Repository) GetByID(id string) (*models.User, error) {
	u := &models.User{}
	err := r.db.QueryRow(
		"SELECT id, username, email, password_hash, created_at FROM users WHERE id = ?", id,
	).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (r *Repository) GetByUsername(username string) (*models.User, error) {
	u := &models.User{}
	err := r.db.QueryRow(
		"SELECT id, username, email, password_hash, created_at FROM users WHERE username = ? OR email = ?",
		username, strings.ToLower(username),
	).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}
