package crypt

import (
	"crypto/md5" // #nosec G501 -- required by the original NPC handshake protocol
	"crypto/rand"
	"encoding/hex"
	"math/big"
)

// Generate 32-bit MD5 strings
func Md5(s string) string {
	// #nosec G401 -- protocol compatibility only; Web/API authentication uses HMAC-SHA256.
	h := md5.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

// Generating Random Verification Key
func GetRandomString(l int) string {
	const alphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
	result := make([]byte, l)
	limit := big.NewInt(int64(len(alphabet)))
	for i := 0; i < l; i++ {
		index, err := rand.Int(rand.Reader, limit)
		if err != nil {
			panic("secure random source unavailable: " + err.Error())
		}
		result[i] = alphabet[index.Int64()]
	}
	return string(result)
}
