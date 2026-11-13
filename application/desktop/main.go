package main

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func main() {
	a := newApp()
	mygo.App.WhenReady(func() {
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:           "nulo",
			Width:           1440,
			Height:          900,
			UseContentSize:  true,
			MinWidth:        880,
			MinHeight:       620,
			StateKey:        "nulo.desktop.main",
			TitleBarStyle:   mygo.TitleBarHidden,
			BackgroundColor: "#0a0a0b",
			Content:         ui.View(a.view),
		})
	})
	err := mygo.App.Run()
	a.closeHost()
	if err != nil {
		log.Fatal(err)
	}
}
