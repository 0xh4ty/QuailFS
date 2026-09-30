package integration

import (
	"crypto/rand"
	"github.com/0xh4ty/quailfs/internal/keys"
	"github.com/0xh4ty/quailfs/internal/network"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"testing"
)

func TestClientPeerIDFirstCharacter(t *testing.T) {
	for i := range 1000 {
		recoveryPhrase := keys.GenerateMnemonic()

		mnemonicSeed := keys.DeriveSeed(recoveryPhrase)

		ed25519Seed, err := keys.DeriveEd25519Seed(mnemonicSeed)
		if err != nil {
			t.Fatalf("iteration %d: derive Ed25519 seed: %v", i, err)
		}

		ed25519PrivateKey, _ := keys.DeriveEd25519Keypair(ed25519Seed)

		libp2pPrivateKey, err := crypto.UnmarshalEd25519PrivateKey(ed25519PrivateKey)
		if err != nil {
			t.Fatalf("iteration %d: unmarshal Ed25519 private key: %v", i, err)
		}

		peerID, err := peer.IDFromPrivateKey(libp2pPrivateKey)
		if err != nil {
			t.Fatalf("iteration %d: derive PeerID: %v", i, err)
		}

		if peerID.String()[0] != '1' {
			t.Fatalf("iteration %d: PeerID %q does not start with '1'", i, peerID)
		}
	}
}

func TestNodePeerIDFirstCharacter(t *testing.T) {
	for i := range 1000 {
		privKey, _, err := crypto.GenerateKeyPairWithReader(
			crypto.RSA,
			2048,
			rand.Reader,
		)
		if err != nil {
			t.Fatalf("iteration %d: generate key pair: %v", i, err)
		}

		node, err := network.NewHost(privKey, false, nil)
		if err != nil {
			t.Fatalf("iteration %d: create host: %v", i, err)
		}

		peerID := node.ID()

		if err := node.Close(); err != nil {
			t.Fatalf("iteration %d: close host: %v", i, err)
		}

		if peerID.String()[0] != 'Q' {
			t.Fatalf("iteration %d: PeerID %q does not start with 'Q'", i, peerID)
		}
	}
}
