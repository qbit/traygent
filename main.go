package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"net/http/pprof"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
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

	app := app.NewWithID("traygent")
	window := app.NewWindow("traygent")
	window.Resize(fyne.NewSize(920, 240))

	ctrlQ := &desktop.CustomShortcut{KeyName: fyne.KeyQ, Modifier: fyne.KeyModifierControl}
	ctrlW := &desktop.CustomShortcut{KeyName: fyne.KeyW, Modifier: fyne.KeyModifierControl}
	window.Canvas().AddShortcut(ctrlQ, func(shortcut fyne.Shortcut) {
		app.Quit()
	})
	window.Canvas().AddShortcut(ctrlW, func(shortcut fyne.Shortcut) {
		window.Hide()
	})

	cmds := LoadCommands(*cmdList)
	tagent := Traygent{
		listener: l,
		addChan:  make(chan ssh.PublicKey),
		rmChan:   make(chan string),
		sigReq:   make(chan ssh.PublicKey),
		sigResp:  make(chan bool),
	}

	keyList := widget.NewTable(
		// Length
		func() (int, int) {
			return len(tagent.keys), 4
		},
		// Create
		func() fyne.CanvasObject {
			//return widget.NewLabel("")
			return container.NewStack(widget.NewLabel(""))
		},
		// Update
		func(i widget.TableCellID, o fyne.CanvasObject) {
			ctnr := o.(*fyne.Container)
			content := ctnr.Objects[0].(*widget.Label)

			key := tagent.keys[i.Row]
			pk := key.signer.PublicKey()

			switch i.Col {
			case 0:
				content.SetText(pk.Type())
			case 1:
				content.SetText(ssh.FingerprintSHA256(pk))
			case 2:
				content.SetText(key.comment)
			case 3:
				content.SetText(key.expire.Format("Mon Jan 2 15:04:05 MST 2006"))
			}
		},
	)

	keyList.ShowHeaderColumn = false

	var lockerButton *widget.Button
	lockerButton = widget.NewButton("Lock Agent", func() {
		// TODO: is there a better way?
		if tagent.locked {
			tagent.Unlock([]byte(""))
			lockerButton.SetText("Lock Agent")
		} else {
			tagent.Lock([]byte(""))
			lockerButton.SetText("Unlock Agent")
		}
	})

	app.SetIcon(buildImage(len(tagent.keys), tagent.locked))

	var desk desktop.App
	var ok bool
	if desk, ok = app.(desktop.App); ok {
		m := fyne.NewMenu("traygent",
			fyne.NewMenuItem("Show", func() {
				window.Show()
			}),
			fyne.NewMenuItem("Remove Keys", func() {
				tagent.RemoveAll()
			}),
		)
		desk.SetSystemTrayMenu(m)
	}

	setInfo := func() {
		iconImg := buildImage(len(tagent.keys), tagent.locked)
		app.SetIcon(iconImg)
		desk.SetSystemTrayIcon(iconImg)

		maxType, maxFP, maxCmt, maxExp := tagent.getMaxes()

		typeSize := fyne.MeasureText(maxType, theme.TextSize()+2, fyne.TextStyle{})
		fpSize := fyne.MeasureText(maxFP, theme.TextSize()+2, fyne.TextStyle{})
		cmtSize := fyne.MeasureText(maxCmt, theme.TextSize()+2, fyne.TextStyle{})
		expSize := fyne.MeasureText(maxExp, theme.TextSize()+2, fyne.TextStyle{})

		keyList.SetColumnWidth(0, typeSize.Width)
		keyList.SetColumnWidth(1, fpSize.Width)
		keyList.SetColumnWidth(2, cmtSize.Width)
		keyList.SetColumnWidth(3, expSize.Width)

		keyList.Refresh()
	}

	setInfo()

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

	go func() {
		for {
			select {
			case added := <-tagent.addChan:
				fp := ssh.FingerprintSHA256(added)
				c := cmds.Get("added")
				if c != nil {
					setInfo()
					c.Run(fp)
				}
			case rm := <-tagent.rmChan:
				c := cmds.Get("removed")
				if c != nil {
					setInfo()
					c.Run(rm)
				}
			case pub := <-tagent.sigReq:
				fp := ssh.FingerprintSHA256(pub)
				c := cmds.Get("sign")
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

	window.SetContent(
		container.NewBorder(
			container.New(
				layout.NewHBoxLayout(),
				lockerButton,
				widget.NewButton("Remove Keys", func() {
					tagent.RemoveAll()
				}),
			),
			nil,
			nil,
			nil,
			keyList,
		),
	)
	window.ShowAndRun()
}
