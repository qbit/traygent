package main

import (
	"fmt"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

type privKey struct {
	signer      ssh.Signer
	comment     string
	expireTime  *time.Time
	lifetime    uint32
	pubKey      ssh.PublicKey
	fingerPrint string
	usage       uint32
}

func (p *privKey) String() string {
	pk := p.signer.PublicKey()
	return fmt.Sprintf("%s %s %s %s",
		pk.Type(),
		p.fingerPrint,
		p.comment,
		p.expireTime.Format(expFormat),
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
	key.LifetimeSecs = exp
	p.lifetime = key.LifetimeSecs
	p.expireTime = &t
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
