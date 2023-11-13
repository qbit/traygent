package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

type UI struct {
	app     fyne.App
	window  fyne.Window
	keyList *widget.Table
	desk    desktop.App
}
