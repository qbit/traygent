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

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

var errLocked = errors.New("agent is locked")

const expFormat = "Mon Jan 2 15:04:05 MST 2006"

type privKey struct {
	signer      ssh.Signer
	comment     string
	expire      *time.Time
	pubKey      ssh.PublicKey
	fingerPrint string
}

func NewPrivKey(signer ssh.Signer, key agent.AddedKey) privKey {
	pub := signer.PublicKey()
	pk := privKey{
		signer:      signer,
		comment:     key.Comment,
		pubKey:      pub,
		fingerPrint: ssh.FingerprintSHA256(pub),
	}
	pk.setExpire(key)

	return pk
}

func (p *privKey) String() string {
	pk := p.signer.PublicKey()
	return fmt.Sprintf("%s %s %s %s",
		pk.Type(),
		p.fingerPrint,
		p.comment,
		p.expire.Format(expFormat),
	)
}

func (p *privKey) GetType() string {
	return p.pubKey.Type()
}

func (p *privKey) GetSum() string {
	return p.fingerPrint
}

func (p *privKey) GetComment() string {
	return p.comment
}

func (p *privKey) setExpire(key agent.AddedKey) {
	exp := key.LifetimeSecs
	if exp <= 0 {
		exp = 300
	}

	t := time.Now().Add(time.Duration(exp) * time.Second)
	p.expire = &t
}

// Traygent extends x/crypto/ssh/agent to hook into fyne for various tasks:
// - notifications
// - allowing UI elements to represent keys
type Traygent struct {
	app     fyne.App
	window  fyne.Window
	keyList *widget.Table
	desk    desktop.App

	expire   uint32
	listener net.Listener
	mu       sync.RWMutex

	keys       []privKey
	locked     bool
	passphrase []byte
}

func NewTraygent() agent.Agent {
	return &Traygent{
		expire: 360,
	}
}

func (t *Traygent) log(title, msgFmt string, msg ...any) {
	fmt.Println("log")
	msgStr := fmt.Sprintf(msgFmt, msg...)

	log.Println(msgStr)

	// TODO: fyne can't send vanishing notifications..
	//notif := fyne.NewNotification(title, msgStr)
	//t.app.SendNotification(notif)
}

func (t *Traygent) remove(key ssh.PublicKey) error {
	fmt.Println("remove")
	hasKey := false

	for i := 0; i < len(t.keys); {
		if bytes.Equal(
			t.keys[i].signer.PublicKey().Marshal(),
			key.Marshal(),
		) {
			hasKey = true

			t.keys[i] = t.keys[len(t.keys)-1]
			t.keys = t.keys[:len(t.keys)-1]

			t.log("Key removed", "removed key: %q\n", ssh.FingerprintSHA256(key))

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
	fmt.Println("RemoveLocked")

	t.mu.Lock()
	defer t.mu.Unlock()

	for _, k := range t.keys {
		if k.expire != nil && time.Now().After(*k.expire) {
			t.remove(k.signer.PublicKey())
		}
	}
}

func (t *Traygent) List() ([]*agent.Key, error) {
	fmt.Println("List")
	t.mu.RLock()
	defer t.mu.RUnlock()

	var pubKeys []*agent.Key
	if t.locked {
		return nil, nil
	}

	for _, k := range t.keys {
		pubKeys = append(pubKeys, &agent.Key{
			Blob:    k.pubKey.Marshal(),
			Comment: fmt.Sprintf("%s [%s]", k.comment, k.expire.Format(expFormat)),
			Format:  k.pubKey.Type(),
		})
	}

	return pubKeys, nil
}

func (t *Traygent) passphrasePrompt(isLock bool, doneFunc func([]byte)) {
	fmt.Println("passphrasePrompt")
	btnStr := "Unlock"
	titleStr := "Unlock Agent"
	if isLock {
		btnStr = "Lock"
		titleStr = "Lock Agent"
	}
	passphrase := widget.NewPasswordEntry()
	items := []*widget.FormItem{
		widget.NewFormItem("Passphrase", passphrase),
	}
	dialog.ShowForm(titleStr, btnStr, "Cancel", items, func(b bool) {
		if !b {
			return
		}

		doneFunc([]byte(passphrase.Text))
	}, t.window)
}

func (t *Traygent) Lock(unused []byte) error {
	fmt.Println("Lock")
	t.log("Agent locked", "locking agent")

	if t.locked {
		return errLocked
	}

	t.passphrasePrompt(true, func(passphrase []byte) {
		t.mu.Lock()
		defer t.mu.Unlock()

		t.locked = true
		t.passphrase = passphrase

		t.Resize()
	})

	return nil
}

func (t *Traygent) Unlock(unused []byte) error {
	fmt.Println("Unlock")
	log.Println("unlocking agent")

	if !t.locked {
		return errors.New("not locked")
	}

	t.passphrasePrompt(true, func(passphrase []byte) {
		t.mu.Lock()
		defer t.mu.Unlock()

		log.Println("hur")
		if subtle.ConstantTimeCompare(passphrase, t.passphrase) == 1 {
			t.locked = false
			t.passphrase = nil

			t.Resize()
		}
	})

	return nil
}

func (t *Traygent) Sign(key ssh.PublicKey, data []byte) (*ssh.Signature, error) {
	fmt.Println("Sign")
	return t.SignWithFlags(key, data, 0)
}

func (t *Traygent) SignWithFlags(key ssh.PublicKey, data []byte, flags agent.SignatureFlags) (*ssh.Signature, error) {
	fmt.Println("SignWithFlags")
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
					return algSiger.SignWithAlgorithm(rand.Reader, data, alg)
				}
			}
		}
	}
	return nil, errors.New("not found")
}

func (t *Traygent) Signers() ([]ssh.Signer, error) {
	fmt.Println("Signers")
	log.Println("signers")

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

func (t *Traygent) getMaxes() (string, string, string, string) {
	fmt.Println("getMaxes")

	t.mu.RLock()
	defer t.mu.RUnlock()

	maxType := ""
	maxSum := ""
	maxComment := ""
	for _, entry := range t.keys {
		if len(entry.GetType()) > len(maxType) {
			maxType = entry.GetType()
		}
		if len(entry.GetSum()) > len(maxSum) {
			maxSum = entry.GetSum()
		}
		if len(entry.GetComment()) > len(maxComment) {
			maxComment = entry.GetComment()
		}

	}

	return maxType, maxSum, maxComment, expFormat
}

func (t *Traygent) Resize() {
	fmt.Println("Resize")
	t.mu.RLock()
	defer t.mu.RUnlock()

	maxType, maxFP, maxCmt, maxExp := t.getMaxes()

	typeSize := fyne.MeasureText(maxType, theme.TextSize()+2, fyne.TextStyle{})
	fpSize := fyne.MeasureText(maxFP, theme.TextSize()+2, fyne.TextStyle{})
	cmtSize := fyne.MeasureText(maxCmt, theme.TextSize()+2, fyne.TextStyle{})
	expSize := fyne.MeasureText(maxExp, theme.TextSize()+2, fyne.TextStyle{})

	t.keyList.SetColumnWidth(0, typeSize.Width)
	t.keyList.SetColumnWidth(1, fpSize.Width)
	t.keyList.SetColumnWidth(2, cmtSize.Width)
	t.keyList.SetColumnWidth(3, expSize.Width)

	iconImg := buildImage(len(t.keys), t.locked)
	t.desk.SetSystemTrayIcon(iconImg)
}

func (t *Traygent) Add(key agent.AddedKey) error {
	fmt.Println("Add")
	t.mu.Lock()

	signer, err := ssh.NewSignerFromKey(key.PrivateKey)
	if err != nil {
		return err
	}

	p := NewPrivKey(signer, key)

	t.keys = append(t.keys, p)
	t.log("Key added", "added %q to agent", p.fingerPrint)

	t.mu.Unlock()
	t.Resize()

	return nil
}

func (t *Traygent) RemoveAll() error {
	fmt.Println("RemoveAll")
	t.mu.Lock()

	if t.locked {
		return errLocked
	}

	t.keys = nil

	t.log("All keys removed", "removed all keys from agent")

	t.mu.Unlock()
	t.Resize()

	return nil
}

func (t *Traygent) Remove(key ssh.PublicKey) error {
	fmt.Println("Remove")
	t.mu.Lock()

	if t.locked {
		return errLocked
	}

	err := t.remove(key)

	t.log("Key removed", "remove key from agent")

	t.mu.Unlock()

	t.Resize()

	return err
}
