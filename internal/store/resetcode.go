package store

import "time"

// SetEmail sets (or clears) a user's email address, used as the destination
// for password-reset codes.
func (s *Store) SetEmail(userID int64, email string) error {
	_, err := s.DB.Exec("UPDATE users SET email=? WHERE id=?", email, userID)
	return err
}

// EmailByID returns a user's stored email ("" when none).
func (s *Store) EmailByID(userID int64) (string, error) {
	var email string
	err := s.DB.QueryRow("SELECT COALESCE(email,'') FROM users WHERE id=?", userID).Scan(&email)
	return email, err
}

// LookupForReset finds a user by username OR email (case-insensitive on email)
// and returns their id + stored email. ok is false when no such user exists or
// they have no email on file — the caller should respond identically in both
// cases so the endpoint never reveals which usernames/emails exist.
func (s *Store) LookupForReset(identifier string) (userID int64, email string, ok bool) {
	row := s.DB.QueryRow(
		"SELECT id, email FROM users WHERE username=? OR (email<>'' AND lower(email)=lower(?)) LIMIT 1",
		identifier, identifier)
	if err := row.Scan(&userID, &email); err != nil {
		return 0, "", false
	}
	if email == "" {
		return 0, "", false
	}
	return userID, email, true
}

// CreateResetCode stores the hash of a freshly-issued reset code. now is passed
// in so callers control the clock (and tests stay deterministic).
func (s *Store) CreateResetCode(userID int64, codeHash string, now time.Time, ttl time.Duration) error {
	_, err := s.DB.Exec(
		"INSERT INTO password_reset_codes(user_id, code_hash, expires_at, used, created_at) VALUES(?,?,?,0,?)",
		userID, codeHash, now.Add(ttl).Unix(), now.Unix())
	return err
}

// UseResetCode consumes a reset code: it must belong to userID, match codeHash,
// be unused, and not be expired at now. On success it marks the code used and
// returns nil; otherwise ErrNotFound. Single-use is enforced atomically via the
// used flag in the UPDATE's WHERE clause.
func (s *Store) UseResetCode(userID int64, codeHash string, now time.Time) error {
	res, err := s.DB.Exec(
		"UPDATE password_reset_codes SET used=1 WHERE user_id=? AND code_hash=? AND used=0 AND expires_at>=?",
		userID, codeHash, now.Unix())
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// PurgeExpiredResetCodes deletes used or expired codes. Best-effort housekeeping.
func (s *Store) PurgeExpiredResetCodes(now time.Time) {
	_, _ = s.DB.Exec("DELETE FROM password_reset_codes WHERE used=1 OR expires_at<?", now.Unix())
}
