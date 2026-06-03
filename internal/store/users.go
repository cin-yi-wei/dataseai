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
