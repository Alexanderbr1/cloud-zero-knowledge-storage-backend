package srp

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"math/big"
	"strings"
	"testing"
)

// clientEphemeral generates a random SRP client ephemeral pair (a, A).
func clientEphemeral(t *testing.T) (a, A *big.Int) {
	t.Helper()
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		t.Fatal(err)
	}
	a = new(big.Int).SetBytes(buf)
	A = new(big.Int).Exp(bigG, a, bigN)
	return
}

// clientSessionKey replicates the client-side S = (B - k·g^x)^(a+u·x) mod N
// and returns K = H(pad(S)).
func clientSessionKey(a, A, x, B *big.Int) []byte {
	hu := sha256.New()
	hu.Write(pad(A))
	hu.Write(pad(B))
	u := new(big.Int).SetBytes(hu.Sum(nil))

	// (B - k·g^x) mod N  ≡  g^b mod N  because B = (k·v + g^b) mod N and v = g^x
	kgx := new(big.Int).Mul(bigK, new(big.Int).Exp(bigG, x, bigN))
	kgx.Mod(kgx, bigN)
	base := new(big.Int).Sub(B, kgx)
	base.Mod(base, bigN)

	exp := new(big.Int).Add(a, new(big.Int).Mul(u, x))
	S := new(big.Int).Exp(base, exp, bigN)
	k := sha256.Sum256(pad(S))
	return k[:]
}

// TestSRPFullRoundTrip runs a complete SRP-6a handshake between an in-process
// client simulation and the real ServerSession, verifying that M1 is accepted
// and M2 is returned.
func TestSRPFullRoundTrip(t *testing.T) {
	// Pick a random SRP private key x and derive the verifier v = g^x mod N.
	xBuf := make([]byte, 32)
	rand.Read(xBuf)
	x := new(big.Int).SetBytes(xBuf)
	v := new(big.Int).Exp(bigG, x, bigN)
	verifierHex := hex.EncodeToString(v.Bytes())

	saltBuf := make([]byte, 16)
	rand.Read(saltBuf)
	saltHex := hex.EncodeToString(saltBuf)
	username := "alice@example.com"

	// Server: init session.
	sess, err := NewServerSession(verifierHex)
	if err != nil {
		t.Fatalf("NewServerSession: %v", err)
	}

	// Client: compute A, K, M1.
	a, A := clientEphemeral(t)
	K := clientSessionKey(a, A, x, sess.B)
	m1 := srpComputeM1(A, sess.B, K, username, saltBuf)
	aHex := hex.EncodeToString(pad(A))
	m1Hex := hex.EncodeToString(m1)

	// Server: verify M1, receive M2.
	m2Hex, err := sess.VerifyClientProof(aHex, m1Hex, username, saltHex)
	if err != nil {
		t.Fatalf("VerifyClientProof: %v", err)
	}
	if m2Hex == "" {
		t.Fatal("expected non-empty M2")
	}
	// Client verifies M2 independently to confirm the server computed it correctly.
	expectedM2 := hex.EncodeToString(srpComputeM2(A, m1, K))
	if m2Hex != expectedM2 {
		t.Errorf("M2 mismatch: server=%s client=%s", m2Hex, expectedM2)
	}
}

// TestSRPSnapshotRoundTrip ensures that a session serialised via Snapshot and
// reconstructed via NewServerSessionFromSnapshot behaves identically.
func TestSRPSnapshotRoundTrip(t *testing.T) {
	xBuf := make([]byte, 32)
	rand.Read(xBuf)
	x := new(big.Int).SetBytes(xBuf)
	v := new(big.Int).Exp(bigG, x, bigN)

	original, err := NewServerSession(hex.EncodeToString(v.Bytes()))
	if err != nil {
		t.Fatal(err)
	}

	vHex, bHex, BHex := original.Snapshot()
	restored, err := NewServerSessionFromSnapshot(vHex, bHex, BHex)
	if err != nil {
		t.Fatalf("NewServerSessionFromSnapshot: %v", err)
	}

	if original.PublicEphemeralHex() != restored.PublicEphemeralHex() {
		t.Fatal("B mismatch after snapshot restore")
	}

	// Full handshake with the restored session must also succeed.
	saltBuf := make([]byte, 16)
	rand.Read(saltBuf)
	saltHex := hex.EncodeToString(saltBuf)
	username := "bob@example.com"

	a, A := clientEphemeral(t)
	K := clientSessionKey(a, A, x, restored.B)
	m1 := srpComputeM1(A, restored.B, K, username, saltBuf)

	if _, err := restored.VerifyClientProof(
		hex.EncodeToString(pad(A)),
		hex.EncodeToString(m1),
		username, saltHex,
	); err != nil {
		t.Fatalf("VerifyClientProof with restored session: %v", err)
	}
}

// TestSRPRejectAEqualsZeroModN checks that A = N (≡ 0 mod N) is rejected
// per the SRP-6a safety rule.
func TestSRPRejectAEqualsZeroModN(t *testing.T) {
	v := new(big.Int).Exp(bigG, big.NewInt(42), bigN)
	sess, err := NewServerSession(hex.EncodeToString(v.Bytes()))
	if err != nil {
		t.Fatal(err)
	}

	aHex := hex.EncodeToString(bigN.Bytes()) // A = N → A mod N == 0
	_, err = sess.VerifyClientProof(aHex, strings.Repeat("00", 32), "u@x.com", strings.Repeat("00", 16))
	if err != ErrInvalidProof {
		t.Fatalf("want ErrInvalidProof for A≡0, got %v", err)
	}
}

// TestSRPRejectAEqualsTwoN checks that A = 2*N (another ≡ 0 mod N case) is rejected.
func TestSRPRejectAEqualsTwoN(t *testing.T) {
	v := new(big.Int).Exp(bigG, big.NewInt(42), bigN)
	sess, err := NewServerSession(hex.EncodeToString(v.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	twoN := new(big.Int).Mul(bigN, big.NewInt(2))
	aHex := hex.EncodeToString(twoN.Bytes())
	_, err = sess.VerifyClientProof(aHex, strings.Repeat("00", 32), "u@x.com", strings.Repeat("00", 16))
	if err != ErrInvalidProof {
		t.Fatalf("want ErrInvalidProof for A=2*N, got %v", err)
	}
}

// TestSRPRejectWrongM1 ensures an incorrect client proof is rejected.
func TestSRPRejectWrongM1(t *testing.T) {
	v := new(big.Int).Exp(bigG, big.NewInt(42), bigN)
	sess, err := NewServerSession(hex.EncodeToString(v.Bytes()))
	if err != nil {
		t.Fatal(err)
	}

	_, A := clientEphemeral(t)
	_, err = sess.VerifyClientProof(
		hex.EncodeToString(pad(A)),
		strings.Repeat("00", 32), // wrong M1
		"u@x.com",
		strings.Repeat("00", 16),
	)
	if err != ErrInvalidProof {
		t.Fatalf("want ErrInvalidProof for wrong M1, got %v", err)
	}
}

// TestSRPInvalidHexInputs verifies that malformed hex is rejected gracefully.
func TestSRPInvalidHexInputs(t *testing.T) {
	t.Run("invalid verifier hex", func(t *testing.T) {
		if _, err := NewServerSession("not-hex!!"); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("invalid snapshot verifier", func(t *testing.T) {
		if _, err := NewServerSessionFromSnapshot("zz", "00", "00"); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("invalid snapshot b", func(t *testing.T) {
		if _, err := NewServerSessionFromSnapshot("00", "zz", "00"); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("invalid snapshot B", func(t *testing.T) {
		if _, err := NewServerSessionFromSnapshot("00", "00", "zz"); err == nil {
			t.Fatal("expected error")
		}
	})
}
