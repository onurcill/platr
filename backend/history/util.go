package history

import (
	"crypto/rand"
	"encoding/hex"
)

func generateID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
