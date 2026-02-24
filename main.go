package main

import (
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"path"
	"syscall"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/driver/desktop"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func init() {
	syscall.Umask(0077)
}

func main() {
	sock := flag.String("s", path.Join(os.Getenv("HOME"), ".traygent"), "Socket path to create")
	cmdList := flag.String("c", "/etc/traygent.json", "List of commands to execute")
	flag.Parse()

	os.Remove(*sock)

	l, err := net.Listen("unix", *sock)
	if err != nil {
		log.Fatalln(err)
	}
	defer l.Close()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	go func(c chan os.Signal) {
		s := <-c
		log.Printf("caught %q, shutting down...\n", s)
		os.Remove(*sock)
		os.Exit(0)
	}(sig)

	cfg := LoadConfig(*cmdList)
	tagent := Traygent{
		listener: l,
		addChan:  make(chan ssh.PublicKey),
		rmChan:   make(chan string),
		sigReq:   make(chan ssh.PublicKey),
		sigResp:  make(chan bool),
	}

	trayApp := app.NewWithID("com.bolddaemon.traygent")

	app.SetMetadata(fyne.AppMetadata{
		Name: "traygent",
	})

	// Create connections to the external agents
	connectExternalAgents := func() {
		tagent.mu.Lock()
		defer tagent.mu.Unlock()

		tagent.externalAgents = []agent.Agent{}
		for _, agentSock := range cfg.ExtraAgents {
			log.Printf("creating a connection for %q", agentSock)
			conn, err := net.Dial("unix", agentSock)
			if err != nil {
				log.Printf("Failed to open SSH_AUTH_SOCK: %v .. skipping", err)
				continue
			}

			client := agent.NewClient(conn)
			tagent.externalAgents = append(tagent.externalAgents, client)
		}
	}

	var desk desktop.App
	var ok bool
	if desk, ok = trayApp.(desktop.App); ok {
		m := fyne.NewMenu("traygent",
			fyne.NewMenuItem("Remove Keys", func() {
				tagent.RemoveAll()
			}),
			fyne.NewMenuItem("Connect external agents", func() {
				connectExternalAgents()
			}),
		)
		desk.SetSystemTrayMenu(m)
	}
	setIcon := func() {
		keys, err := tagent.List()
		if err != nil {
			return
		}
		iconImg := buildImage(len(keys), tagent.locked)
		desk.SetSystemTrayIcon(iconImg)
	}

	setIcon()

	go func() {
		for {
			tagent.RemoveLocked()
			time.Sleep(1 * time.Second)
		}
	}()

	connectExternalAgents()

	go func() {
		for {
			c, err := tagent.listener.Accept()
			if err != nil {
				continue
			}

			go agent.ServeAgent(&tagent, c)
		}
	}()

	go func() {
		for {
			select {
			case added := <-tagent.addChan:
				fp := ssh.FingerprintSHA256(added)
				c := cfg.Get("added")
				if c != nil {
					setIcon()
					c.Run(fp)
				}
			case rm := <-tagent.rmChan:
				c := cfg.Get("removed")
				if c != nil {
					setIcon()
					c.Run(rm)
				}
			case pub := <-tagent.sigReq:
				fp := ssh.FingerprintSHA256(pub)
				c := cfg.Get("sign")
				if c != nil {
					if c.Run(fp) {
						go func() { tagent.sigResp <- true }()
					} else {
						go func() { tagent.sigResp <- false }()
					}
				}
			}
		}
	}()

	trayApp.Run()
}
