package install

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"

	sourcegithub "github.com/inherelab/eget/internal/source/github"
)

type noVerifier struct{}

type sha256Error struct {
	Expected []byte
	Got      []byte
}

type sha256Verifier struct {
	Expected []byte
}

type sha256Printer struct{}

type sha256AssetVerifier struct {
	AssetURL string
	Getter   sourcegithub.HTTPGetter
}

func (n *noVerifier) Verify(b []byte) error {
	return nil
}

func (e *sha256Error) Error() string {
	return fmt.Sprintf("sha256 checksum mismatch:\nexpected: %x\ngot:      %x", e.Expected, e.Got)
}

func newSha256Verifier(expectedHex string) (*sha256Verifier, error) {
	expected, _ := hex.DecodeString(expectedHex)
	if len(expected) != sha256.Size {
		return nil, fmt.Errorf("sha256sum (%s) too small: %d bytes decoded", expectedHex, len(expectedHex))
	}
	return &sha256Verifier{Expected: expected}, nil
}

func (s *sha256Verifier) Verify(b []byte) error {
	sum := sha256.Sum256(b)
	if bytes.Equal(sum[:], s.Expected) {
		return nil
	}
	return &sha256Error{Expected: s.Expected, Got: sum[:]}
}

func (s *sha256Printer) Verify(b []byte) error {
	sum := sha256.Sum256(b)
	fmt.Printf("%x\n", sum)
	return nil
}

func (s *sha256AssetVerifier) Verify(b []byte) error {
	if s.Getter == nil {
		return fmt.Errorf("github getter is required")
	}
	resp, err := s.Getter.Get(s.AssetURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	expected := make([]byte, sha256.Size)
	n, err := hex.Decode(expected, data)
	if n < sha256.Size {
		return fmt.Errorf("sha256sum (%s) too small: %d bytes decoded", string(data), n)
	}
	sum := sha256.Sum256(b)
	if bytes.Equal(sum[:], expected[:n]) {
		return nil
	}
	return &sha256Error{Expected: expected[:n], Got: sum[:]}
}
