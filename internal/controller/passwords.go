package controller

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"

	"golang.org/x/crypto/argon2"
)

type hashedPassword struct {
	Hash string
	Salt string
}

func hashPassword(password string) (*hashedPassword, error) {
	return rehashPassword(password, newSalt())
}

func verifyPassword(password, salt, expectedHash string) (bool, error) {
	hashed, err := rehashPassword(password, salt)
	if err != nil {
		return false, err
	}
	return hashed.Hash == expectedHash, nil
}

func rehashPassword(password, salt string) (*hashedPassword, error) {
	s, err := base64.StdEncoding.DecodeString(salt)
	if err != nil {
		return nil, fmt.Errorf("decode password salt: %w", err)
	}
	hash := argon2.IDKey([]byte(password), s, 1, 3*1024, 4, 32)
	return &hashedPassword{
		Hash: base64.StdEncoding.EncodeToString(hash),
		Salt: salt,
	}, nil
}

func newSalt() string {
	buf := make([]byte, binary.MaxVarintLen64)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return base64.StdEncoding.EncodeToString(buf)
}
