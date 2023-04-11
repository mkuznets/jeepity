package jeepity

import (
	"encoding/base64"
	"fmt"
	"mkuznets.com/go/jeepity/internal/store"
	"mkuznets.com/go/ytils/ycrypto"
)

type Cryptor interface {
	EncryptMessage(user *store.User, message *store.Message) error
	DecryptMessage(user *store.User, message *store.Message) error
}

type aesEncryptor struct {
	password string
}

func NewAesEncryptor(password string) Cryptor {
	return &aesEncryptor{password: password}
}

func (e *aesEncryptor) EncryptMessage(user *store.User, message *store.Message) error {
	key := ycrypto.EncryptionKey(e.password, user.Salt)

	encrypted, err := ycrypto.Encrypt([]byte(message.Message), key)
	if err != nil {
		return fmt.Errorf("encrypt: %w", err)
	}

	message.Message = base64.StdEncoding.EncodeToString(encrypted)
	message.Version = store.MessageVersionV2
	return nil
}

func (e *aesEncryptor) DecryptMessage(user *store.User, message *store.Message) error {
	key := ycrypto.EncryptionKey(e.password, user.Salt)

	switch message.Version {
	case store.MessageVersionV2:
		encrypted, err := base64.StdEncoding.DecodeString(message.Message)
		if err != nil {
			return fmt.Errorf("base64 decode: %w", err)
		}
		decrypted, err := ycrypto.Decrypt(encrypted, key)
		if err != nil {
			return fmt.Errorf("decrypt: %w", err)
		}
		message.Message = string(decrypted)
	default:
		return fmt.Errorf("unsupported message version: %d", message.Version)
	}
	return nil
}
