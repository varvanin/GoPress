package store

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
)

type User struct {
	ID, Version, CreatedAt int64
	Email, Name, Role      string
	Blocked                bool
}
type actorKey struct{}

func WithActor(ctx context.Context, user User) context.Context {
	return context.WithValue(ctx, actorKey{}, user)
}
func Actor(ctx context.Context) (User, bool) { u, ok := ctx.Value(actorKey{}).(User); return u, ok }
func Owner(ctx context.Context) int64 {
	if u, ok := Actor(ctx); ok {
		return u.ID
	}
	return 1
}

var ErrLastAdmin = errors.New("at least one active administrator is required")

func (s *Store) User(id int64) (User, error) {
	var u User
	err := s.DB.QueryRow("SELECT id,email,name,role,blocked,version,created_at FROM users WHERE id=?", id).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.Blocked, &u.Version, &u.CreatedAt)
	return u, err
}
func (s *Store) UserByEmail(email string) (User, error) {
	var id int64
	if err := s.DB.QueryRow("SELECT id FROM users WHERE email=? AND blocked=0", email).Scan(&id); err != nil {
		return User{}, err
	}
	return s.User(id)
}
func (s *Store) SessionUser(hash string) (User, error) {
	var id int64
	if err := s.DB.QueryRow("SELECT u.id FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.hash=? AND s.expires_at>unixepoch() AND u.blocked=0", hash).Scan(&id); err != nil {
		return User{}, err
	}
	return s.User(id)
}
func (s *Store) Users(page int) ([]User, int, error) {
	var total int
	if err := s.DB.QueryRow("SELECT count(*) FROM users").Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.DB.Query("SELECT id,email,name,role,blocked,version,created_at FROM users ORDER BY id LIMIT 25 OFFSET ?", (page-1)*25)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	users := []User{}
	for rows.Next() {
		var u User
		if err = rows.Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.Blocked, &u.Version, &u.CreatedAt); err != nil {
			return nil, 0, err
		}
		users = append(users, u)
	}
	return users, total, rows.Err()
}
func auditTx(ctx context.Context, tx *sql.Tx, action, object string) error {
	u, _ := Actor(ctx)
	_, err := tx.ExecContext(ctx, "INSERT INTO audit_events(user_id,action,object) VALUES(?,?,?)", u.ID, action, object)
	return err
}
func (s *Store) Audit(ctx context.Context, action, object string) error {
	u, _ := Actor(ctx)
	_, err := s.DB.ExecContext(ctx, "INSERT INTO audit_events(user_id,action,object) VALUES(?,?,?)", u.ID, action, object)
	return err
}
func (s *Store) SaveUser(ctx context.Context, u User, passwordHash []byte) (int64, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var duplicate bool
	if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE email=? COLLATE NOCASE AND id!=?)", u.Email, u.ID).Scan(&duplicate); err != nil {
		return 0, err
	}
	if duplicate {
		return 0, ErrConflict
	}
	if u.ID == 0 {
		res, err := tx.ExecContext(ctx, "INSERT INTO users(email,name,role,blocked,password_hash) VALUES(?,?,?,?,?)", u.Email, u.Name, u.Role, u.Blocked, passwordHash)
		if err != nil {
			return 0, err
		}
		u.ID, err = res.LastInsertId()
		if err != nil {
			return 0, err
		}
	} else {
		var role string
		var blocked bool
		if err = tx.QueryRowContext(ctx, "SELECT role,blocked FROM users WHERE id=?", u.ID).Scan(&role, &blocked); err != nil {
			return 0, err
		}
		if role == "admin" && !blocked && (u.Role != "admin" || u.Blocked) {
			var n int
			if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM users WHERE role='admin' AND blocked=0 AND id!=?", u.ID).Scan(&n); err != nil {
				return 0, err
			}
			if n == 0 {
				return 0, ErrLastAdmin
			}
		}
		res, err := tx.ExecContext(ctx, "UPDATE users SET email=?,name=?,role=?,blocked=?,password_hash=coalesce(?,password_hash),version=version+1 WHERE id=? AND version=?", u.Email, u.Name, u.Role, u.Blocked, passwordHash, u.ID, u.Version)
		if err != nil {
			return 0, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return 0, err
		}
		if n != 1 {
			return 0, ErrConflict
		}
		if _, err = tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id=?", u.ID); err != nil {
			return 0, err
		}
		if _, err = tx.ExecContext(ctx, "DELETE FROM password_resets WHERE user_id=?", u.ID); err != nil {
			return 0, err
		}
	}
	if err = auditTx(ctx, tx, "user.save", strconv.FormatInt(u.ID, 10)); err != nil {
		return 0, err
	}
	return u.ID, tx.Commit()
}
func (s *Store) ChangePassword(ctx context.Context, id int64, expectedHash, newHash []byte) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	query := "UPDATE users SET password_hash=?,version=version+1 WHERE id=? AND blocked=0"
	args := []any{newHash, id}
	if expectedHash != nil {
		query += " AND password_hash=?"
		args = append(args, expectedHash)
	}
	res, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrConflict
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id=?", id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM password_resets WHERE user_id=?", id); err != nil {
		return err
	}
	if err = auditTx(ctx, tx, "user.password", strconv.FormatInt(id, 10)); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) CreateReset(id int64, hash string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("DELETE FROM password_resets WHERE user_id=? OR expires_at<=unixepoch()", id); err != nil {
		return err
	}
	if _, err = tx.Exec("INSERT INTO password_resets(hash,user_id,expires_at) VALUES(?,?,unixepoch()+1800)", hash, id); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) ResetPassword(ctx context.Context, hash string, newHash []byte) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id int64
	if err = tx.QueryRowContext(ctx, "SELECT r.user_id FROM password_resets r JOIN users u ON u.id=r.user_id WHERE r.hash=? AND r.expires_at>unixepoch() AND u.blocked=0", hash).Scan(&id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE users SET password_hash=?,version=version+1 WHERE id=?", newHash, id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id=?", id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM password_resets WHERE user_id=?", id); err != nil {
		return err
	}
	if err = auditTx(ctx, tx, "user.reset", strconv.FormatInt(id, 10)); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) MediaOwner(name string) (int64, error) {
	var id int64
	err := s.DB.QueryRow("SELECT owner_id FROM media WHERE name=?", name).Scan(&id)
	return id, err
}

func (s *Store) LoginSession(hash string, userID int64, passwordHash []byte) error {
	res, err := s.DB.Exec("INSERT INTO sessions(hash,expires_at,user_id) SELECT ?,unixepoch()+43200,id FROM users WHERE id=? AND blocked=0 AND password_hash=?", hash, userID, passwordHash)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n != 1 {
		return ErrConflict
	}
	return err
}
