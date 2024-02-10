package lib

import (
	"fmt"
	"log"
)

const (
	color_red = uint8(iota + 91)
	color_green
	color_yellow
	color_blue
	color_magenta //洋红
	info = "[INFO]"
	trac = "[TRAC]"
	erro = "[ERRO]"
	warn = "[WARN]"
	succ = "[SUCC]"
)

func XLogInfo(objs...interface{}){
	text := ""
	for i, obj := range objs {
		if i == 0 {
			text += fmt.Sprintf("%v", obj)
		} else {
			text += fmt.Sprintf(", %v", obj)
		}
	}
	log.Printf("\x1b[%dmINFO: %s\x1b[0m", color_yellow, text)
}

func XLogErr(objs...interface{}){
	text := ""
	for i, obj := range objs {
		if i == 0 {
			text += fmt.Sprintf("%v", obj)
		} else {
			text += fmt.Sprintf(", %v", obj)
		}
	}
	log.Printf("\x1b[%dmERR: %s\x1b[0m", color_red, text)
}
