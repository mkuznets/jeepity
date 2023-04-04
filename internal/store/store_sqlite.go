package store

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/jmoiron/sqlx"
	"mkuznets.com/go/jeepity/sql/sqlite"
	"mkuznets.com/go/ytils/ycrypto"
	"mkuznets.com/go/ytils/ytime"

	// Required to load "sqlite" driver
	_ "github.com/mattn/go-sqlite3"
)

type storeSqlite struct {
	db          *sqlx.DB
	initialised bool
}

func (s *storeSqlite) init(ctx context.Context) error {
	if s.initialised {
		return nil
	}
	content, err := sqlite.FS.ReadFile("schema.sql")
	if err != nil {
		return err
	}

	if _, err := s.db.ExecContext(ctx, string(content)); err != nil {
		return err
	}
	s.initialised = true
	return nil
}

func NewSqliteStoreFromPath(path string) Store {
	dsn := "file:" + path + "?cache=shared&mode=rwc&_journal_mode=WAL&_synchronous=NORMAL&_writable_schema=0&_foreign_keys=1&_txlock=immediate"
	db := sqlx.MustConnect("sqlite3", dsn)
	return &storeSqlite{db: db}
}

func (s *storeSqlite) GetUser(ctx context.Context, chatId int64) (*User, error) {
	if err := s.init(ctx); err != nil {
		return nil, err
	}

	var user User
	query := `SELECT chat_id, approved, username, full_name, created_at, updated_at FROM users WHERE chat_id = ?`
	if err := s.db.GetContext(ctx, &user, query, chatId); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

func (s *storeSqlite) PutUser(ctx context.Context, user *User) error {
	if err := s.init(ctx); err != nil {
		return err
	}

	u := *user
	u.CreatedAt = ytime.Now()
	u.UpdatedAt = ytime.Now()

	query := `
	INSERT INTO users (chat_id, approved, username, full_name, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?)
	ON CONFLICT DO NOTHING`

	_, err := s.db.ExecContext(ctx, query, u.ChatId, u.Approved, u.Username, u.FullName, u.CreatedAt, u.UpdatedAt)
	return err
}

func (s *storeSqlite) ApproveUser(ctx context.Context, chatId int64) error {
	if err := s.init(ctx); err != nil {
		return err
	}

	query := `UPDATE users SET approved = true and updated_at = ? WHERE chat_id = ?`
	_, err := s.db.ExecContext(ctx, query, ytime.Now(), chatId)
	return err
}

func (s *storeSqlite) GetDialogMessages(ctx context.Context, chatId int64) ([]*Message, error) {
	if err := s.init(ctx); err != nil {
		return nil, err
	}

	query := `
	SELECT id, chat_id, role, message, created_at
	FROM messages
	WHERE chat_id = ?
	ORDER BY id ASC`

	var messages []*Message
	if err := s.db.SelectContext(ctx, &messages, query, chatId); err != nil {
		return nil, err
	}

	for _, msg := range messages {
		obscuredMsg, err := ycrypto.Reveal(msg.Message)
		if err != nil {
			return nil, fmt.Errorf("message reveal: %w", err)
		}
		msg.Message = obscuredMsg
	}

	return messages, nil
}

func (s *storeSqlite) PutMessages(ctx context.Context, messages []*Message) error {
	if err := s.init(ctx); err != nil {
		return err
	}

	return doTx(ctx, s.db, func(tx *sqlx.Tx) error {
		query := `
		INSERT INTO messages (chat_id, role, message, created_at)
		VALUES (?, ?, ?, ?)`

		for _, msg := range messages {
			m := *msg
			m.CreatedAt = ytime.Now()
			obscuredMsg, err := ycrypto.Obscure(m.Message)
			if err != nil {
				return err
			}
			m.Message = obscuredMsg

			if _, err := tx.ExecContext(ctx, query, m.ChatId, m.Role, m.Message, m.CreatedAt); err != nil {
				return err
			}
		}

		return nil
	})
}

func (s *storeSqlite) ClearMessages(ctx context.Context, chatId int64) error {
	if err := s.init(ctx); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM messages WHERE chat_id = ?`, chatId)
	return err
}

func (s *storeSqlite) PutUsage(ctx context.Context, usage *Usage) error {
	if err := s.init(ctx); err != nil {
		return err
	}

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
