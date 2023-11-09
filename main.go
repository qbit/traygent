package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func main() {
	dbg := flag.Bool("debug", false, "Enable pprof debugging")
	sock := flag.String("s", "/tmp/traygent", "Socket path to create")
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

	for {
		select {
		case added := <-tagent.addChan:
			fmt.Printf("NOTICE: added %q\n", ssh.FingerprintSHA256(added))
		case rm := <-tagent.rmChan:
			fmt.Printf("NOTICE: removed %q\n", rm)
		case pub := <-tagent.sigReq:
			r := bufio.NewReader(os.Stdin)

			fmt.Printf("NOTICE: Allow access to %q?: ", ssh.FingerprintSHA256(pub))
			resp, _ := r.ReadString('\n')
			resp = strings.Trim(resp, "\n")

			if resp == "yes" {
				go func() { tagent.sigResp <- true }()
			} else {
				go func() { tagent.sigResp <- false }()
			}
			fmt.Printf("%q\n", resp)
		}
	}
}
