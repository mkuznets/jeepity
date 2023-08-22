package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/oklog/ulid/v2"
	"golang.org/x/exp/slog"
	"mkuznets.com/go/ytils/ylog"
	"mkuznets.com/go/ytils/yrand"
	"mkuznets.com/go/ytils/ytime"
	"ytils.dev/sqlite-migrator"

	"mkuznets.com/go/jeepity/internal/ybot"
	"mkuznets.com/go/jeepity/sql/sqlite"

	// Required to load "sqlite" driver.
	_ "github.com/mattn/go-sqlite3"
)

const (
	DialogRetention = time.Hour
	SaltLength      = 32
)

type SqliteStore struct {
	defaultInviteCode string
	db                *sqlx.DB
}

func (s *SqliteStore) init(ctx context.Context) error {
	m := migrator.New(s.db.DB, sqlite.FS)
	m.WithLogFunc(func(msg string, args ...interface{}) {
		slog.Debug(msg, args...)
	})
	return m.Migrate(ctx)
}

func (s *SqliteStore) Close() {
	if _, err := s.db.Exec("vacuum"); err != nil {
		slog.Error("sqlite vacuum", ylog.Err(err))
	}
	if err := s.db.Close(); err != nil {
		slog.Error("sqlite close", ylog.Err(err))
	}
}

func (s *SqliteStore) SetDefaultInviteCode(code string) {
	s.defaultInviteCode = code
}

func NewSqlite(path string) (*SqliteStore, error) {
	dsn := "file:" + path + "?cache=shared&mode=rwc&_journal_mode=WAL&_synchronous=EXTRA&_writable_schema=0&_foreign_keys=1&_txlock=immediate"
	db := sqlx.MustConnect("sqlite3", dsn)

	s := &SqliteStore{db: db}
	if err := s.init(context.Background()); err != nil {
		return nil, fmt.Errorf("sqlite store init: %w", err)
	}

	return s, nil
}

func (s *SqliteStore) GetUser(ctx context.Context, chatId int64) (*User, error) {
	var user User
	query := `
	SELECT
	    chat_id,
	    approved,
	    username,
	    full_name,
	    salt,
	    coalesce(model, '') as model,
	    coalesce(invite_code, '') as invite_code,
	    coalesce(system_prompt, '') as system_prompt,
	    coalesce(input_state, '') as input_state,
	    coalesce(dialog_id, '') as dialog_id,
	    created_at,
	    updated_at
	FROM users WHERE chat_id = ?`
	if err := s.db.GetContext(ctx, &user, query, chatId); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // nolint:nilnil // nil value is used upstream
		}
		return nil, err
	}

	return &user, nil
}

// SetSystemPrompt sets the system prompt for the user.
func (s *SqliteStore) SetSystemPrompt(ctx context.Context, chatId int64, prompt string) error {
	return doTx(ctx, s.db, func(tx *sqlx.Tx) error {
		query := `UPDATE users SET system_prompt = ? WHERE chat_id = ?`
		_, err := tx.ExecContext(ctx, query, prompt, chatId)
		if err != nil {
			return fmt.Errorf("sql: UPDATE system_prompt: %w", err)
		}
		return nil
	})
}

func (s *SqliteStore) SetInputState(ctx context.Context, chatId int64, state InputState) error {
	return doTx(ctx, s.db, func(tx *sqlx.Tx) error {
		query := `UPDATE users SET input_state = ? WHERE chat_id = ?`
		_, err := tx.ExecContext(ctx, query, state, chatId)
		if err != nil {
			return fmt.Errorf("sql: UPDATE input_state: %w", err)
		}
		return nil
	})
}

// EnsureInviteCode checks if the user has an invite code and generates a new one if not.
func (s *SqliteStore) EnsureInviteCode(ctx context.Context, user *User) error {
	if user.InviteCode != "" {
		return nil
	}
	user.InviteCode = ybot.InviteCode()

	return doTx(ctx, s.db, func(tx *sqlx.Tx) error {
		query := `UPDATE users SET invite_code = ? WHERE chat_id = ?`
		_, err := tx.ExecContext(ctx, query, user.InviteCode, user.ChatId)
		if err != nil {
			return fmt.Errorf("EnsureInviteCode: %w", err)
		}
		return nil
	})
}

func (s *SqliteStore) EnsureDiglogID(ctx context.Context, user *User) error {
	if user.DialogID != "" {
		return nil
	}
	user.DialogID = newDialogID()

	return doTx(ctx, s.db, func(tx *sqlx.Tx) error {
		query := `UPDATE users SET dialog_id = ? WHERE chat_id = ?`
		_, err := tx.ExecContext(ctx, query, user.DialogID, user.ChatId)
		if err != nil {
			return fmt.Errorf("EnsureDiglogID: %w", err)
		}
		return nil
	})
}

func (s *SqliteStore) ResetDiglogID(ctx context.Context, user *User) error {
	user.DialogID = ""
	return s.EnsureDiglogID(ctx, user)
}

func (s *SqliteStore) CheckInviteCode(ctx context.Context, user *User, code string) error {
	return doTx(ctx, s.db, func(tx *sqlx.Tx) error {
		var invitedBy int64

		if s.defaultInviteCode != "" && code == s.defaultInviteCode {
			invitedBy = 0
		} else {
			err := s.db.Get(&invitedBy, "SELECT chat_id FROM users WHERE invite_code = ? LIMIT 1", code)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return nil
				}
				return fmt.Errorf("CheckInviteCode: %w", err)
			}
		}

		user.Approved = true

		query := `UPDATE users SET invited_by = ?, approved=1 WHERE chat_id = ?`
		if _, err := tx.ExecContext(ctx, query, invitedBy, user.ChatId); err != nil {
			return fmt.Errorf("CheckInviteCode: %w", err)
		}
		return nil
	})
}

func (s *SqliteStore) PutUser(ctx context.Context, user *User) (*User, error) {
	u := *user
	u.Salt = yrand.Base62(SaltLength)
	u.CreatedAt = ytime.Now()
	u.UpdatedAt = ytime.Now()
	u.InviteCode = ybot.InviteCode()
	u.DialogID = newDialogID()

	query := `
	INSERT INTO users (chat_id, approved, username, full_name, created_at, updated_at, salt, model, invite_code, dialog_id)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT DO NOTHING`

	_, err := s.db.ExecContext(ctx, query, u.ChatId, u.Approved, u.Username, u.FullName, u.CreatedAt, u.UpdatedAt, u.Salt, "", u.InviteCode, u.DialogID)

	return &u, err
}

func (s *SqliteStore) ApproveUser(ctx context.Context, chatId int64) error {
	query := `UPDATE users SET approved = true and updated_at = ? WHERE chat_id = ?`
	_, err := s.db.ExecContext(ctx, query, ytime.Now(), chatId)
	return err
}

func (s *SqliteStore) GetDialogMessages(ctx context.Context, chatId int64) ([]*Message, error) {
	retentionQuery := `SELECT COUNT(*) FROM messages WHERE created_at > ? AND chat_id = ?`
	var recentMessages int
	retentionThreshold := ytime.New(time.Now().Add(-DialogRetention))

	if err := s.db.QueryRowxContext(ctx, retentionQuery, retentionThreshold, chatId).Scan(&recentMessages); err != nil {
		return nil, err
	}

	if recentMessages == 0 {
		if err := s.ClearMessages(ctx, chatId); err != nil {
			return nil, fmt.Errorf("ClearMessages: %w", err)
		}
		return nil, nil
	}

	dialogQuery := `
	SELECT id, chat_id, role, message, created_at, version
	FROM messages
	WHERE chat_id = ?
	ORDER BY id ASC`

	var messages []*Message
	if err := s.db.SelectContext(ctx, &messages, dialogQuery, chatId); err != nil {
		return nil, err
	}
	return messages, nil
}

func (s *SqliteStore) PutMessages(ctx context.Context, messages []*Message) error {
	return doTx(ctx, s.db, func(tx *sqlx.Tx) error {
		query := `
		INSERT INTO messages (chat_id, role, message, created_at, version)
		VALUES (?, ?, ?, ?, ?)`

		for _, msg := range messages {
			m := *msg
			m.CreatedAt = ytime.Now()
			if _, err := tx.ExecContext(ctx, query, m.ChatId, m.Role, m.Message, m.CreatedAt, m.Version); err != nil {
				return err
			}
		}

		return nil
	})
}

func (s *SqliteStore) ClearMessages(ctx context.Context, chatId int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM messages WHERE chat_id = ?`, chatId)
	return err
}

func (s *SqliteStore) PutUsage(ctx context.Context, usage *Usage) error {
	u := *usage
	u.CreatedAt = ytime.Now()

	query := `
	INSERT INTO usage (chat_id, update_id, model, completion_tokens, prompt_tokens, total_tokens, created_at)
	VALUES (?, ?, ?, ?, ?, ?, ?)`

	_, err := s.db.ExecContext(
		ctx, query,
		u.ChatId, u.UpdateId, u.Model, u.CompletionTokens, u.PromptTokens, u.TotalTokens, u.CreatedAt,
	)
	return err
}

func doTx(ctx context.Context, db *sqlx.DB, op func(tx *sqlx.Tx) error) error {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if err := op(tx); err != nil {
		return err
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func newDialogID() string {
	return "dia_" + ulid.Make().String()
}
