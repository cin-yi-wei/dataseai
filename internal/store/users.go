package store

import (
	"database/sql"
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrDuplicate          = errors.New("duplicate")
	ErrNotFound           = errors.New("not found")
	ErrInvalidCredentials = errors.New("invalid credentials")
)

// dummyBcryptHash is used to defeat user-enumeration timing attacks on
// VerifyPassword. CompareHashAndPassword against this hash takes the same
// ~80ms as a real bcrypt check; the result is discarded.
//
// This is bcrypt.GenerateFromPassword([]byte("never-matches-anything"), 10).
// Generated once offline; baked into source so startup stays fast.
const dummyBcryptHash = "$2a$10$gNiXr6e1P/9/fPGvCRjxbuTd9eVYCcxBiE8ZfwYxQEZEKWAph0hzm"

type Store struct {
	DB *sql.DB
}

type User struct {
	ID       int64
	Username string
}

func (s *Store) CreateUser(username, password string) (User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, err
	}
	res, err := s.DB.Exec("INSERT INTO users(username, password_hash) VALUES(?, ?)", username, string(hash))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return User{}, ErrDuplicate
		}
		return User{}, err
	}
	id, _ := res.LastInsertId()
	return User{ID: id, Username: username}, nil
}

func (s *Store) GetUserByID(id int64) (User, error) {
	row := s.DB.QueryRow("SELECT id, username FROM users WHERE id=?", id)
	var u User
	if err := row.Scan(&u.ID, &u.Username); err != nil {
		if err == sql.ErrNoRows {
			return u, ErrNotFound
		}
		return u, err
	}
	return u, nil
}

func (s *Store) VerifyPassword(username, password string) (User, error) {
	row := s.DB.QueryRow("SELECT id, username, password_hash FROM users WHERE username=?", username)
	var u User
	var hash string
	if err := row.Scan(&u.ID, &u.Username, &hash); err != nil {
		if err == sql.ErrNoRows {
			// Run bcrypt against a dummy hash so unknown-user timing
			// matches the wrong-password path. Defeats user enumeration.
			_ = bcrypt.CompareHashAndPassword([]byte(dummyBcryptHash), []byte(password))
			return u, ErrInvalidCredentials
		}
		return u, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return User{}, ErrInvalidCredentials
	}
	return u, nil
}

func (s *Store) UpdatePassword(userID int64, newPassword string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec("UPDATE users SET password_hash=? WHERE id=?", string(hash), userID)
	return err
}
