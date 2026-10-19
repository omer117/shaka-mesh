package service

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"strings"
)

const rsaKeyBits = 2048

// KeySet is the set of RSA keys auth-service knows about: one private key that
// signs new tokens and any number of retired public keys that are still
// published (and accepted) so tokens signed before a rotation stay valid until
// they expire.
type KeySet struct {
	signing   *rsa.PrivateKey
	signingID string
	public    map[string]*rsa.PublicKey // every published key, including the signing key
	order     []string                  // kids in publication order, signing key first
}

// NewKeySet builds a KeySet. previous may be empty.
func NewKeySet(signing *rsa.PrivateKey, previous ...*rsa.PublicKey) (*KeySet, error) {
	if signing == nil {
		return nil, errors.New("signing key is required")
	}
	if signing.N.BitLen() < rsaKeyBits {
		return nil, fmt.Errorf("RSA key must be at least %d bits", rsaKeyBits)
	}
	set := &KeySet{signing: signing, public: map[string]*rsa.PublicKey{}}
	set.signingID = KeyID(&signing.PublicKey)
	set.add(set.signingID, &signing.PublicKey)
	for _, key := range previous {
		if key.N.BitLen() < rsaKeyBits {
			return nil, fmt.Errorf("previous RSA key must be at least %d bits", rsaKeyBits)
		}
		set.add(KeyID(key), key)
	}
	return set, nil
}

func (s *KeySet) add(kid string, key *rsa.PublicKey) {
	if _, exists := s.public[kid]; exists {
		return
	}
	s.public[kid] = key
	s.order = append(s.order, kid)
}

// GenerateKeySet creates a throw-away signing key. It is for local development
// only: the key vanishes on restart and differs per replica.
func GenerateKeySet() (*KeySet, error) {
	key, err := rsa.GenerateKey(rand.Reader, rsaKeyBits)
	if err != nil {
		return nil, fmt.Errorf("generate RSA key: %w", err)
	}
	return NewKeySet(key)
}

// SigningKey returns the private key and its kid.
func (s *KeySet) SigningKey() (*rsa.PrivateKey, string) { return s.signing, s.signingID }

// PublicKey looks up a published key by kid.
func (s *KeySet) PublicKey(kid string) (*rsa.PublicKey, bool) {
	key, ok := s.public[kid]
	return key, ok
}

// JWK is one entry of a JSON Web Key Set (RFC 7517) for an RSA signing key.
type JWK struct {
	KeyType   string `json:"kty"`
	Use       string `json:"use"`
	Algorithm string `json:"alg"`
	KeyID     string `json:"kid"`
	N         string `json:"n"`
	E         string `json:"e"`
}

// JWKS is the document served at /.well-known/jwks.json.
type JWKS struct {
	Keys []JWK `json:"keys"`
}

// JWKS returns the public keys, signing key first. Private material never
// appears here.
func (s *KeySet) JWKS() JWKS {
	keys := make([]JWK, 0, len(s.order))
	for _, kid := range s.order {
		keys = append(keys, publicJWK(kid, s.public[kid]))
	}
	return JWKS{Keys: keys}
}

func publicJWK(kid string, key *rsa.PublicKey) JWK {
	return JWK{
		KeyType:   "RSA",
		Use:       "sig",
		Algorithm: "RS256",
		KeyID:     kid,
		N:         base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
		E:         base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
	}
}

// KeyID derives a stable kid from the public key: the RFC 7638 JWK thumbprint
// (SHA-256 over the canonical JSON of e, kty, n), base64url-encoded. The same
// key therefore has the same kid in every process and after every restart, and
// no kid has to be configured by hand.
func KeyID(key *rsa.PublicKey) string {
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())
	n := base64.RawURLEncoding.EncodeToString(key.N.Bytes())
	canonical := fmt.Sprintf(`{"e":"%s","kty":"RSA","n":"%s"}`, e, n)
	sum := sha256.Sum256([]byte(canonical))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// ParsePrivateKeyPEM accepts PKCS#8 ("PRIVATE KEY") or PKCS#1 ("RSA PRIVATE
// KEY") PEM. Literal "\n" sequences are expanded so a key can live in a
// single-line environment variable.
func ParsePrivateKeyPEM(text string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(normalizePEM(text)))
	if block == nil {
		return nil, errors.New("no PEM block found in private key")
	}
	switch block.Type {
	case "PRIVATE KEY":
		parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse PKCS#8 private key: %w", err)
		}
		key, ok := parsed.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("private key is not an RSA key")
		}
		return key, nil
	case "RSA PRIVATE KEY":
		key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse PKCS#1 private key: %w", err)
		}
		return key, nil
	default:
		return nil, fmt.Errorf("unsupported PEM block %q for the private key", block.Type)
	}
}

// ParsePublicKeysPEM reads one or more "PUBLIC KEY" (PKIX) or "RSA PUBLIC KEY"
// (PKCS#1) blocks, for retired keys that must stay published after a rotation.
func ParsePublicKeysPEM(text string) ([]*rsa.PublicKey, error) {
	rest := []byte(normalizePEM(text))
	var keys []*rsa.PublicKey
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		switch block.Type {
		case "PUBLIC KEY":
			parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("parse public key: %w", err)
			}
			key, ok := parsed.(*rsa.PublicKey)
			if !ok {
				return nil, errors.New("public key is not an RSA key")
			}
			keys = append(keys, key)
		case "RSA PUBLIC KEY":
			key, err := x509.ParsePKCS1PublicKey(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("parse PKCS#1 public key: %w", err)
			}
			keys = append(keys, key)
		default:
			return nil, fmt.Errorf("unsupported PEM block %q for a public key", block.Type)
		}
	}
	if len(keys) == 0 {
		return nil, errors.New("no PEM public key found")
	}
	return keys, nil
}

func normalizePEM(text string) string {
	text = strings.TrimSpace(text)
	if !strings.Contains(text, "\n") {
		text = strings.ReplaceAll(text, `\n`, "\n")
	}
	return text
}
