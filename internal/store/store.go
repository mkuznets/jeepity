package store

import (
	"context"
	"mkuznets.com/go/ytils/ytime"
)

type User struct {
	ChatId   int64  `db:"chat_id"`
	Approved bool   `db:"approved"`
	Username string `db:"username"`
	FullName string `db:"full_name"`

	CreatedAt ytime.Time `db:"created_at"`
	UpdatedAt ytime.Time `db:"updated_at"`
}

type Message struct {
	Id        int64      `db:"id"`
	ChatId    int64      `db:"chat_id"`
	Role      string     `db:"role"`
	Message   string     `db:"message"`
	CreatedAt ytime.Time `db:"created_at"`
}

type Usage struct {
	Id               int        `db:"id"`
	ChatId           int64      `db:"chat_id"`
	UpdateId         int        `db:"update_id"`
	Model            string     `db:"model"`
	CompletionTokens int        `db:"completion_tokens"`
	PromptTokens     int        `db:"prompt_tokens"`
	TotalTokens      int        `db:"total_tokens"`
	CreatedAt        ytime.Time `db:"created_at"`
}

type Store interface {
	GetUser(ctx context.Context, chatId int64) (*User, error)
	PutUser(ctx context.Context, user *User) error
	ApproveUser(ctx context.Context, chatId int64) error

	GetMessages(ctx context.Context, chatId int64) ([]*Message, error)
	PutMessages(ctx context.Context, message []*Message) error
	ClearMessages(ctx context.Context, chatId int64) error

	PutUsage(ctx context.Context, usage *Usage) error
}
