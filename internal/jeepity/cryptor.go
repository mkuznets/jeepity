package jeepity

import (
	"fmt"
	"mkuznets.com/go/ytils/ybase"
	"mkuznets.com/go/ytils/ycrypto"
)

type Cryptor interface {
	Encrypt(salt, data string) (string, error)
	Decrypt(salt, data string) (string, error)
}

type aesEncryptor struct {
	password string
}

func NewAesEncryptor(password string) Cryptor {
	return &aesEncryptor{password: password}
}

func (e *aesEncryptor) Encrypt(salt, data string) (string, error) {
	key := ycrypto.EncryptionKey(e.password, salt)
	encrypted, err := ycrypto.Encrypt([]byte(data), key)
	if err != nil {
		return "", fmt.Errorf("encrypt: %w", err)
	}
	return ybase.EncodeBase62(encrypted), nil
}

func (e *aesEncryptor) Decrypt(salt, data string) (string, error) {
	key := ycrypto.EncryptionKey(e.password, salt)
	encrypted := ybase.DecodeBase62(data)
	decrypted, err := ycrypto.Decrypt(encrypted, key)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(decrypted), nil
}
