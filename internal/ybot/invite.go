package ybot

import (
	"crypto/rand"
	"fmt"
	"math/big"

	"mkuznets.com/go/ytils/y"
)

const (
	maxInviteCode = 99999999
)

func InviteUrl(username, code string) string {
	return fmt.Sprintf("https://t.me/%s?start=%s", username, code)
}

func InviteCode() string {
	nBig := y.Must(rand.Int(rand.Reader, big.NewInt(maxInviteCode)))
	return fmt.Sprintf("%08d", nBig.Int64())
}
