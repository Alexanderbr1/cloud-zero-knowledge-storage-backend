package srp

import (
	"crypto/rand"
	"encoding/hex"
	"math/big"
	"testing"
)

// prepareVerifier returns a random verifier hex string — cheap, excludes
// the client-side x/v derivation from the benchmark under test.
func prepareVerifier(b *testing.B) string {
	b.Helper()
	xBuf := make([]byte, 32)
	if _, err := rand.Read(xBuf); err != nil {
		b.Fatal(err)
	}
	x := new(big.Int).SetBytes(xBuf)
	v := new(big.Int).Exp(bigG, x, bigN)
	return hex.EncodeToString(v.Bytes())
}

// BenchmarkNewServerSession measures the cost of creating a server session:
// one modexp (g^b mod N) per login-init call.
func BenchmarkNewServerSession(b *testing.B) {
	verifierHex := prepareVerifier(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := NewServerSession(verifierHex); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkVerifyClientProof measures the cost of verifying M1 and computing M2:
// one modexp (S = (A·v^u)^b mod N) per login-finalize call.
// Sessions and matching proofs are pre-generated to isolate verify from init.
func BenchmarkVerifyClientProof(b *testing.B) {
	const pool = 64
	type entry struct {
		sess    *ServerSession
		aHex    string
		m1Hex   string
		saltHex string
	}
	entries := make([]entry, pool)
	for i := range entries {
		xBuf := make([]byte, 32)
		rand.Read(xBuf)
		x := new(big.Int).SetBytes(xBuf)
		v := new(big.Int).Exp(bigG, x, bigN)

		sess, err := NewServerSession(hex.EncodeToString(v.Bytes()))
		if err != nil {
			b.Fatal(err)
		}

		saltBuf := make([]byte, 16)
		rand.Read(saltBuf)

		a, A := clientEphemeral(b)
		K := clientSessionKey(a, A, x, sess.B)
		m1 := srpComputeM1(A, sess.B, K, "bench@example.com", saltBuf)

		entries[i] = entry{
			sess:    sess,
			aHex:    hex.EncodeToString(pad(A)),
			m1Hex:   hex.EncodeToString(m1),
			saltHex: hex.EncodeToString(saltBuf),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e := entries[i%pool]
		if _, err := e.sess.VerifyClientProof(e.aHex, e.m1Hex, "bench@example.com", e.saltHex); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkFullHandshake measures the end-to-end server cost of one SRP login:
// NewServerSession (login/init) + VerifyClientProof (login/finalize).
func BenchmarkFullHandshake(b *testing.B) {
	xBuf := make([]byte, 32)
	rand.Read(xBuf)
	x := new(big.Int).SetBytes(xBuf)
	v := new(big.Int).Exp(bigG, x, bigN)
	verifierHex := hex.EncodeToString(v.Bytes())

	saltBuf := make([]byte, 16)
	rand.Read(saltBuf)
	saltHex := hex.EncodeToString(saltBuf)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sess, err := NewServerSession(verifierHex)
		if err != nil {
			b.Fatal(err)
		}
		a, A := clientEphemeral(b)
		K := clientSessionKey(a, A, x, sess.B)
		m1 := srpComputeM1(A, sess.B, K, "bench@example.com", saltBuf)

		if _, err := sess.VerifyClientProof(
			hex.EncodeToString(pad(A)),
			hex.EncodeToString(m1),
			"bench@example.com", saltHex,
		); err != nil {
			b.Fatal(err)
		}
	}
}
