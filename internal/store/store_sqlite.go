package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"time"

	"github.com/jmoiron/sqlx"
	"golang.org/x/exp/slog"
	"mkuznets.com/go/ytils/yrand"
	"mkuznets.com/go/ytils/ytime"

	"mkuznets.com/go/jeepity/sql/sqlite"

	// Required to load "sqlite" driver.
	_ "github.com/mattn/go-sqlite3"
)

const (
	DialogRetention = time.Hour
	SaltLength      = 32
)

var reMigrationFilename = regexp.MustCompile(`^(\d+)_.*\.sql$`)

type SqliteStore struct {
	db *sqlx.DB
}

type migrationFile struct {
	id       uint32
	filename string
	sql      string
}

func (s *SqliteStore) init(ctx context.Context) error {
	var currentVersion uint32
	if err := s.db.GetContext(ctx, &currentVersion, "PRAGMA user_version"); err != nil {
		return fmt.Errorf("init schema: %w", err)
	}

	entries, err := sqlite.FS.ReadDir(".")
	if err != nil {
		return err
	}

	migrations := make([]*migrationFile, 0, len(entries))
	for _, entry := range entries {
		id, err := getMigrationId(entry.Name())
		if err != nil {
			return err
		}

		content, err := sqlite.FS.ReadFile(entry.Name())
		if err != nil {
			return err
		}

		if id <= currentVersion {
			continue
		}

		migrations = append(migrations, &migrationFile{
			id:       id,
			filename: entry.Name(),
			sql:      string(content),
		})
	}

	if len(migrations) == 0 {
		return nil
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].id < migrations[j].id
	})

	err = doTx(ctx, s.db, func(tx *sqlx.Tx) error {
		for _, migration := range migrations {
			if _, err := tx.ExecContext(ctx, migration.sql); err != nil {
				return fmt.Errorf("init schema: %w", err)
			}

			slog.Debug("migration applied", "id", migration.id, "filename", migration.filename)
		}

		pragmaQuery := fmt.Sprintf("PRAGMA user_version = %d", migrations[len(migrations)-1].id)
		if _, err := tx.ExecContext(ctx, pragmaQuery); err != nil {
			return fmt.Errorf("PRAGMA: %w", err)
		}

		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

func (s *SqliteStore) Close() {
	if _, err := s.db.Exec("vacuum"); err != nil {
		slog.Error("sqlite vacuum", err)
	}
	if err := s.db.Close(); err != nil {
		slog.Error("sqlite close", err)
	}
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
	query := `SELECT chat_id, approved, username, full_name, salt, coalesce(model, '') as model, created_at, updated_at FROM users WHERE chat_id = ?`
	if err := s.db.GetContext(ctx, &user, query, chatId); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // nolint:nilnil // nil value is used upstream
		}
		return nil, err
	}

	return &user, nil
}

func (s *SqliteStore) PutUser(ctx context.Context, user *User) (*User, error) {
	u := *user
	u.Salt = yrand.Base62(SaltLength)
	u.CreatedAt = ytime.Now()
	u.UpdatedAt = ytime.Now()

	query := `
	INSERT INTO users (chat_id, approved, username, full_name, created_at, updated_at, salt, model)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT DO NOTHING`

	_, err := s.db.ExecContext(ctx, query, u.ChatId, u.Approved, u.Username, u.FullName, u.CreatedAt, u.UpdatedAt, u.Salt, "")

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

func getMigrationId(filename string) (uint32, error) {
	matches := reMigrationFilename.FindStringSubmatch(filename)
	if len(matches) < 2 { // nolint:gomnd // matches[0] is the full string
		return 0, fmt.Errorf("parse migration filename: %s", filename)
	}
	idTime, err := time.Parse("20060102150405", matches[1])
	if err != nil {
		return 0, err
	}
	epoch := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
	return uint32(idTime.Unix() - epoch), nil
}
