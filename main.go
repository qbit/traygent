package main

import (
	"flag"
	"fmt"
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
	"fyne.io/fyne/v2/widget"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func main() {
	dbg := flag.Bool("debug", false, "Enable pprof debugging")
	flag.Parse()

	if *dbg {
		go func() {
			mux := http.NewServeMux()
			mux.HandleFunc("/", pprof.Profile)
			log.Fatal(http.ListenAndServe(":7777", mux))
		}()
	}

	l, err := net.Listen("unix", "/tmp/traygent")
	if err != nil {
		log.Fatalln(err)
	}
	tagent := Traygent{
		app:      app.NewWithID("traygent"),
		listener: l,
	}

	tagent.window = tagent.app.NewWindow("traygent")
	tagent.window.Resize(fyne.NewSize(920, 240))

	ctrlQ := &desktop.CustomShortcut{KeyName: fyne.KeyQ, Modifier: fyne.KeyModifierControl}
	ctrlW := &desktop.CustomShortcut{KeyName: fyne.KeyW, Modifier: fyne.KeyModifierControl}
	tagent.window.Canvas().AddShortcut(ctrlQ, func(shortcut fyne.Shortcut) {
		tagent.app.Quit()
	})
	tagent.window.Canvas().AddShortcut(ctrlW, func(shortcut fyne.Shortcut) {
		tagent.window.Hide()
	})

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

	tagent.keyList = widget.NewTable(
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

	tagent.keyList.ShowHeaderColumn = false

	iconImg := buildImage(len(tagent.keys), tagent.locked)
	tagent.app.SetIcon(iconImg)

	if desk, ok := tagent.app.(desktop.App); ok {
		tagent.desk = desk
		m := fyne.NewMenu("traygent",
			fyne.NewMenuItem("Show", func() {
				tagent.window.Show()
			}),
			fyne.NewMenuItem("Remove Keys", func() {
				tagent.RemoveAll()
			}),
		)
		tagent.desk.SetSystemTrayMenu(m)
		tagent.desk.SetSystemTrayIcon(iconImg)
		tagent.app.SetIcon(iconImg)
	}

	go func() {
		for {
			tagent.RemoveLocked()
			time.Sleep(1 * time.Second)
		}
	}()

	var lockerButton = widget.NewButton("Lock Agent", func() {
		fmt.Println("clicked", tagent.locked)
		// TODO: is there a better way?
		//if tagent.locked {
		//	tagent.Unlock([]byte(""))
		//	lockerButton.SetText("Lock Agent")
		//} else {
		//	tagent.Lock([]byte(""))
		//	lockerButton.SetText("Unlock Agent")
		//}
	})

	tagent.window.SetContent(
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
			tagent.keyList,
		),
	)
	tagent.window.ShowAndRun()
}
