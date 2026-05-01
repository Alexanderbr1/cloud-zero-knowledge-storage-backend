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
	bigN = mustParseBigInt(nHex)
	bigG = big.NewInt(2)
	// bigK = H(pad(N) || pad(g)) — SRP-6a multiplier, pre-computed as *big.Int.
	bigK = computeGroupMultiplier()
	// srpXorNG = H(N) ⊕ H(g) — constant term in the M1 formula, pre-computed.
	srpXorNG = computeXorNG()
)

func mustParseBigInt(h string) *big.Int {
	n, ok := new(big.Int).SetString(h, 16)
	if !ok {
		// Недостижимо: nHex — константа времени компиляции.
		panic("srp: invalid RFC 5054 group prime")
	}
	return n
}

func computeGroupMultiplier() *big.Int {
	h := sha256.New()
	h.Write(pad(bigN))
	h.Write(pad(bigG))
	return new(big.Int).SetBytes(h.Sum(nil))
}

func computeXorNG() []byte {
	hN := sha256.Sum256(pad(bigN))
	hG := sha256.Sum256(pad(bigG))
	out := make([]byte, sha256.Size)
	for i := range out {
		out[i] = hN[i] ^ hG[i]
	}
	return out
}

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
	kv := new(big.Int).Mul(bigK, v)
	kv.Mod(kv, bigN)
	gb := new(big.Int).Exp(bigG, b, bigN)
	B := new(big.Int).Add(kv, gb)
	B.Mod(B, bigN)

	return &ServerSession{verifier: v, b: b, B: B}, nil
}

func (s *ServerSession) PublicEphemeralHex() string {
	return hex.EncodeToString(pad(s.B))
}

// VerifyClientProof validates the client's proof M1 and returns the server proof M2.
//
//   - aHex       — client's public ephemeral A (hex)
//   - m1Hex      — client's proof M1 (hex)
//   - username   — user's email address (UTF-8), used as SRP identity I
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

	K := srpSessionKey(A, s.verifier, s.b, s.B)
	wantM1 := srpComputeM1(A, s.B, K, username, saltBytes)

	gotM1, err := hex.DecodeString(m1Hex)
	if err != nil {
		return "", fmt.Errorf("srp: decode M1: %w", err)
	}
	if subtle.ConstantTimeCompare(wantM1, gotM1) != 1 {
		return "", ErrInvalidProof
	}

	return hex.EncodeToString(srpComputeM2(A, wantM1, K)), nil
}

// srpSessionKey computes the shared session key K = H(pad(S)) where
// S = (A · v^u)^b mod N and u = H(pad(A) || pad(B)).
func srpSessionKey(A, v, b, B *big.Int) []byte {
	hu := sha256.New()
	hu.Write(pad(A))
	hu.Write(pad(B))
	u := new(big.Int).SetBytes(hu.Sum(nil))

	vu := new(big.Int).Exp(v, u, bigN)
	base := new(big.Int).Mul(A, vu)
	base.Mod(base, bigN)
	S := new(big.Int).Exp(base, b, bigN)

	k := sha256.Sum256(pad(S))
	return k[:]
}

// srpComputeM1 computes M1 = H(H(N)⊕H(g) || H(I) || salt || pad(A) || pad(B) || K).
func srpComputeM1(A, B *big.Int, K []byte, username string, saltBytes []byte) []byte {
	hI := sha256.Sum256([]byte(username))
	h := sha256.New()
	h.Write(srpXorNG)
	h.Write(hI[:])
	h.Write(saltBytes)
	h.Write(pad(A))
	h.Write(pad(B))
	h.Write(K)
	return h.Sum(nil)
}

// srpComputeM2 computes M2 = H(pad(A) || M1 || K).
func srpComputeM2(A *big.Int, M1, K []byte) []byte {
	h := sha256.New()
	h.Write(pad(A))
	h.Write(M1)
	h.Write(K)
	return h.Sum(nil)
}
