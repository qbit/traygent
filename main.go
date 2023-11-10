package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"net/http/pprof"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func main() {
	dbg := flag.Bool("debug", false, "Enable pprof debugging")
	sock := flag.String("s", "/tmp/traygent", "Socket path to create")
	cmdList := flag.String("c", "/etc/traygent.json", "List of commands to execute")
	flag.Parse()

	if *dbg {
		go func() {
			mux := http.NewServeMux()
			mux.HandleFunc("/", pprof.Profile)
			log.Fatal(http.ListenAndServe(":7777", mux))
		}()
	}

	l, err := net.Listen("unix", *sock)
	if err != nil {
		log.Fatalln(err)
	}
	tagent := Traygent{
		listener: l,
		addChan:  make(chan ssh.PublicKey),
		rmChan:   make(chan string),
		sigReq:   make(chan ssh.PublicKey),
		sigResp:  make(chan bool),
	}

	go func() {
		for {
			tagent.RemoveLocked()
			time.Sleep(1 * time.Second)
		}
	}()

	go func() {
		for {
			c, err := tagent.listener.Accept()
			if err != nil {
				log.Println(err)
				continue
			}

			agent.ServeAgent(&tagent, c)
		}
	}()

	cmds := LoadCommands(*cmdList)

	for {
		select {
		case added := <-tagent.addChan:
			fp := ssh.FingerprintSHA256(added)
			log.Printf("NOTICE: added %q\n", fp)
			c := cmds.Get("added")
			if c != nil {
				c.Run(fp)
			}
		case rm := <-tagent.rmChan:
			log.Printf("NOTICE: removed %q\n", rm)
			c := cmds.Get("removed")
			if c != nil {
				c.Run(rm)
			}
		case pub := <-tagent.sigReq:
			fp := ssh.FingerprintSHA256(pub)
			log.Printf("NOTICE: access request for: %q?\n", fp)
			c := cmds.Get("sign")
			if c != nil {
				if c.Run(fp) {
					go func() { tagent.sigResp <- true }()
				} else {
					go func() { tagent.sigResp <- false }()
				}
			} else {
				panic("nope")
			}
		}
	}
}
