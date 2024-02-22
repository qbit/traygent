package main

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

var errLocked = errors.New("agent is locked")

const expFormat = "Mon Jan 2 15:04:05 MST 2006"

// Traygent extends x/crypto/ssh/agent to hook into fyne for various tasks:
// - notifications
// - allowing UI elements to represent keys
type Traygent struct {
	expire   uint32
	listener net.Listener
	mu       sync.RWMutex

	keys       []privKey
	passphrase []byte
	locked     bool

	addChan chan ssh.PublicKey
	rmChan  chan string
	sigReq  chan ssh.PublicKey
	sigResp chan bool
}

func (t *Traygent) log(title, msgFmt string, msg ...any) {
	msgStr := fmt.Sprintf(msgFmt, msg...)

	log.Println(msgStr)
}

func (t *Traygent) remove(key ssh.PublicKey, reason string) error {
	hasKey := false

	strReason := ""
	switch reason {
	case "expired":
		strReason = "key expired"
	case "request":
		strReason = "user requested key be removed"
	default:
		log.Fatalf("unknown removal reason: %q\n", reason)
	}

	for i := 0; i < len(t.keys); {
		if bytes.Equal(
			t.keys[i].signer.PublicKey().Marshal(),
			key.Marshal(),
		) {
			hasKey = true

			t.keys[i] = t.keys[len(t.keys)-1]
			t.keys = t.keys[:len(t.keys)-1]

			fp := ssh.FingerprintSHA256(key)
			t.log("Key removed", "removed key: %q (%s)\n", fp, strReason)
			go func() { t.rmChan <- fp }()

			continue
		} else {
			i++
		}
	}

	if !hasKey {
		return errors.New("key not found")
	}

	return nil
}

func (t *Traygent) RemoveLocked() {
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, k := range t.keys {
		now := time.Now()

		// Without Round(0) when coming out of S3 suspend the After check below fails
		// https://github.com/golang/go/issues/36141
		now = now.Round(0)
		k.expireTime.Round(0)

		if k.expireTime != nil && now.After(*k.expireTime) {
			t.remove(k.signer.PublicKey(), "expired")
		}
	}
}

func (t *Traygent) List() ([]*agent.Key, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var pubKeys []*agent.Key
	if t.locked {
		return nil, nil
	}

	for _, k := range t.keys {
		pubKeys = append(pubKeys, &agent.Key{
			Blob:    k.pubKey.Marshal(),
			Comment: fmt.Sprintf("%s [%s]", k.comment, k.expireTime.Format(expFormat)),
			Format:  k.pubKey.Type(),
		})
	}

	return pubKeys, nil
}

func (t *Traygent) Lock(passphrase []byte) error {
	t.log("Agent locked", "locking agent")

	if t.locked {
		return errLocked
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.passphrase = passphrase
	t.locked = true

	return nil
}

func (t *Traygent) Unlock(unusedpassphrase []byte) error {

	if t.locked {
		return errors.New("not locked")
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if subtle.ConstantTimeCompare(unusedpassphrase, t.passphrase) == 1 {
		t.passphrase = nil
		t.locked = false
	}

	return nil
}

func (t *Traygent) Sign(key ssh.PublicKey, data []byte) (*ssh.Signature, error) {
	var sig *ssh.Signature

	go func() { t.sigReq <- key }()

	if <-t.sigResp {
		return t.SignWithFlags(key, data, 0)
	}

	return sig, fmt.Errorf("not allowed")
}

func (t *Traygent) SignWithFlags(key ssh.PublicKey, data []byte, flags agent.SignatureFlags) (*ssh.Signature, error) {
	if t.locked {
		return nil, errLocked
	}

	t.RemoveLocked()

	t.mu.Lock()
	defer t.mu.Unlock()

	pk := key.Marshal()
	for _, k := range t.keys {
		if bytes.Equal(k.signer.PublicKey().Marshal(), pk) {
			if flags == 0 {
				return k.signer.Sign(rand.Reader, data)
			} else {
				if algSiger, ok := k.signer.(ssh.AlgorithmSigner); !ok {
					return nil, fmt.Errorf("%T is not supported", k.signer)
				} else {
					var alg string
					switch flags {
					case agent.SignatureFlagRsaSha256:
						alg = ssh.KeyAlgoRSASHA256
					case agent.SignatureFlagRsaSha512:
						alg = ssh.KeyAlgoRSASHA512
					default:
						return nil, fmt.Errorf("unsupported signature flags: %d", flags)
					}
					k.usage++
					return algSiger.SignWithAlgorithm(rand.Reader, data, alg)
				}
			}
		}
	}
	return nil, errors.New("not found")
}

func (t *Traygent) Signers() ([]ssh.Signer, error) {

	if t.locked {
		return nil, errLocked
	}

	t.RemoveLocked()

	t.mu.Lock()
	defer t.mu.Unlock()

	signers := make([]ssh.Signer, 0, len(t.keys))
	for _, k := range t.keys {
		signers = append(signers, k.signer)
	}

	return signers, nil
}

func (t *Traygent) Add(key agent.AddedKey) error {
	signer, err := ssh.NewSignerFromKey(key.PrivateKey)
	if err != nil {
		return err
	}

	p := NewPrivKey(signer, key)

	t.mu.RLock()
	for _, k := range t.keys {
		if bytes.Equal(
			k.signer.PublicKey().Marshal(),
			signer.PublicKey().Marshal(),
		) {
			t.log("Key already added", "key %q already exists in agent with expiration %d", k.fingerPrint, k.lifetime)
			t.mu.RUnlock()
			return nil
		}
	}
	t.mu.RUnlock()

	t.mu.Lock()
	t.keys = append(t.keys, p)
	t.log("Key added", "added %q to agent with expiration %d", p.fingerPrint, p.lifetime)

	go func() { t.addChan <- p.pubKey }()

	t.mu.Unlock()

	return nil
}

func (t *Traygent) RemoveAll() error {

	if t.locked {
		return errLocked
	}

	t.mu.Lock()
	klen := len(t.keys)
	t.keys = nil

	t.log("All keys removed", "removed %d keys from agent", klen)
	go func() { t.rmChan <- "all" }()

	t.mu.Unlock()

	return nil
}

func (t *Traygent) Remove(key ssh.PublicKey) error {

	if t.locked {
		return errLocked
	}

	t.mu.Lock()
	err := t.remove(key, "request")

	t.log("Key removed", "remove key from agent")

	t.mu.Unlock()

	return err
}

func NewTraygent() agent.Agent {
	return &Traygent{
		expire:  360,
		addChan: make(chan ssh.PublicKey),
		rmChan:  make(chan string),
	}
}
