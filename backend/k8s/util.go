package k8s

import (
	"crypto/rand"
	"encoding/hex"
)

func generateID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
