// Package srp implements the SRP-6a (Secure Remote Password) server side
// using the RFC 5054 2048-bit group and SHA-256 as the hash function.
//
// The bcrypt hardening is applied by the caller: the client derives the SRP
// private key as x = H(srpSalt || bcryptHash(password, bcryptSalt)) so that
// an attacker who steals the verifier still needs to run bcrypt per guess.
package srp

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
)

// RFC 5054 §A.1 — 2048-bit group.
const nHex = "ffffffffffffffffc90fdaa22168c234c4c6628b80dc1cd1" +
	"29024e088a67cc74020bbea63b139b22514a08798e3404dd" +
	"ef9519b3cd3a431b302b0a6df25f14374fe1356d6d51c245" +
	"e485b576625e7ec6f44c42e9a637ed6b0bff5cb6f406b7ed" +
	"ee386bfb5a899fa5ae9f24117c4b1fe649286651ece45b3d" +
	"c2007cb8a163bf0598da48361c55d39a69163fa8fd24cf5f" +
	"83655d23dca3ad961c62f356208552bb9ed529077096966d" +
	"670c354e4abc9804f1746c08ca18217c32905e462e36ce3b" +
	"e39e772c180e86039b2783a2ec07a28fb5c55df06f4c52c9" +
	"de2bcbf6955817183995497cea956ae515d2261898fa0510" +
	"15728e5a8aacaa68ffffffffffffffff"

var (
	bigN *big.Int
	bigG *big.Int

	// k = H(pad(N) || pad(g))  — SRP-6a multiplier, precomputed.
	bigK []byte
)

func init() {
	bigN, _ = new(big.Int).SetString(nHex, 16)
	bigG = big.NewInt(2)

	h := sha256.New()
	h.Write(pad(bigN))
	h.Write(pad(bigG))
	bigK = h.Sum(nil)
}

// pad returns x left-zero-padded to the byte length of N.
func pad(x *big.Int) []byte {
	nLen := (bigN.BitLen() + 7) / 8 // 256 bytes for 2048-bit
	xBytes := x.Bytes()
	if len(xBytes) >= nLen {
		return xBytes
	}
	out := make([]byte, nLen)
	copy(out[nLen-len(xBytes):], xBytes)
	return out
}

// ErrInvalidProof is returned when the client's M1 does not match.
var ErrInvalidProof = errors.New("srp: invalid client proof")

// ServerSession holds the ephemeral state of one SRP-6a server handshake.
// Create with NewServerSession; call VerifyClientProof once.
type ServerSession struct {
	verifier *big.Int
	b        *big.Int // private ephemeral
	B        *big.Int // public ephemeral, sent to client
}

// NewServerSession initialises a server session from the stored verifier (hex-encoded).
// It generates a random 256-bit private ephemeral b and computes B = k·v + g^b mod N.
func NewServerSession(verifierHex string) (*ServerSession, error) {
	vBytes, err := hex.DecodeString(verifierHex)
	if err != nil {
		return nil, fmt.Errorf("srp: decode verifier: %w", err)
	}
	v := new(big.Int).SetBytes(vBytes)

	bBytes := make([]byte, 32)
	if _, err := rand.Read(bBytes); err != nil {
		return nil, fmt.Errorf("srp: generate b: %w", err)
	}
	b := new(big.Int).SetBytes(bBytes)

	// B = (k·v + g^b) mod N
	kv := new(big.Int).Mul(new(big.Int).SetBytes(bigK), v)
	kv.Mod(kv, bigN)
	gb := new(big.Int).Exp(bigG, b, bigN)
	B := new(big.Int).Add(kv, gb)
	B.Mod(B, bigN)

	return &ServerSession{verifier: v, b: b, B: B}, nil
}

// PublicEphemeralHex returns the server's public ephemeral B as a padded hex string.
func (s *ServerSession) PublicEphemeralHex() string {
	return hex.EncodeToString(pad(s.B))
}

// VerifyClientProof validates the client's proof M1 and returns the server proof M2.
//
//   - aHex      — client's public ephemeral A (hex)
//   - m1Hex     — client's proof M1 (hex)
//   - username  — user's email address (UTF-8), used as SRP identity I
//   - srpSaltHex — the SRP salt stored for this user (hex)
func (s *ServerSession) VerifyClientProof(aHex, m1Hex, username, srpSaltHex string) (string, error) {
	aBytes, err := hex.DecodeString(aHex)
	if err != nil {
		return "", fmt.Errorf("srp: decode A: %w", err)
	}
	A := new(big.Int).SetBytes(aBytes)

	// SRP safety check: A mod N ≠ 0
	if new(big.Int).Mod(A, bigN).Sign() == 0 {
		return "", ErrInvalidProof
	}

	saltBytes, err := hex.DecodeString(srpSaltHex)
	if err != nil {
		return "", fmt.Errorf("srp: decode salt: %w", err)
	}

	// u = H(pad(A) || pad(B))
	hu := sha256.New()
	hu.Write(pad(A))
	hu.Write(pad(s.B))
	u := new(big.Int).SetBytes(hu.Sum(nil))

	// S = (A · v^u)^b mod N
	vu := new(big.Int).Exp(s.verifier, u, bigN)
	base := new(big.Int).Mul(A, vu)
	base.Mod(base, bigN)
	S := new(big.Int).Exp(base, s.b, bigN)

	// K = H(pad(S))
	hK := sha256.Sum256(pad(S))
	K := hK[:]

	// M1 = H(H(N)⊕H(g) || H(I) || salt || pad(A) || pad(B) || K)
	hN := sha256.Sum256(pad(bigN))
	hG := sha256.Sum256(pad(bigG))
	xorNG := make([]byte, sha256.Size)
	for i := range xorNG {
		xorNG[i] = hN[i] ^ hG[i]
	}
	hI := sha256.Sum256([]byte(username))

	hm1 := sha256.New()
	hm1.Write(xorNG)
	hm1.Write(hI[:])
	hm1.Write(saltBytes)
	hm1.Write(pad(A))
	hm1.Write(pad(s.B))
	hm1.Write(K)
	expectedM1 := hm1.Sum(nil)

	gotM1, err := hex.DecodeString(m1Hex)
	if err != nil {
		return "", fmt.Errorf("srp: decode M1: %w", err)
	}
	if subtle.ConstantTimeCompare(expectedM1, gotM1) != 1 {
		return "", ErrInvalidProof
	}

	// M2 = H(pad(A) || M1 || K)
	hm2 := sha256.New()
	hm2.Write(pad(A))
	hm2.Write(expectedM1)
	hm2.Write(K)

	return hex.EncodeToString(hm2.Sum(nil)), nil
}
