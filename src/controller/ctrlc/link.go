package ctrlc

import (
	"github.com/danieldin95/openlan-go/src/controller/libctrl"
	"github.com/danieldin95/openlan-go/src/libol"
)

type Link struct {
	libctrl.Listen
	cc *CtrlC
}

func (h *Link) AddCtl(id string, m libctrl.Message) error {
	libol.Cmd("Link.AddCtl %s %s", id, m.Data)
	return nil
}

func (h *Link) DelCtl(id string, m libctrl.Message) error {
	libol.Cmd("Link.DelCtl %s %s", id, m.Data)
	return nil
}
