package network

import (
	"fmt"
	"kernelKoala/internal/logger"
	"os"
	"sync"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type ifaceTablePrinter struct {
	chEvent   chan PayLoadTc
	app       *tview.Application
	table     *tview.Table
	tableData map[string][]Event
	mapLock   sync.RWMutex
	layout    *tview.Flex
	header    *tview.TextView
}

// InitUI sets up the terminal dashboard layout with single header + single table.
func (i *ifaceTablePrinter) InitUI() {
	i.chEvent = make(chan PayLoadTc, 100)
	i.app = tview.NewApplication()
	i.tableData = make(map[string][]Event)

	_, _ = os.Hostname()
	i.header = tview.NewTextView()

	i.header.SetDynamicColors(true)
	i.header.SetTextAlign(tview.AlignCenter)

	i.table = tview.NewTable()

	i.layout = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(i.header, 1, 0, false).
		AddItem(i.table, 0, 1, true)

	i.app.SetRoot(i.layout, true).EnableMouse(true)

	go i.forward()
}

func (i *ifaceTablePrinter) forward() {
	for ev := range i.chEvent {
		i.mapLock.Lock()
		logger.Info("%s", ev.Iface)
		i.tableData[ev.Iface] = append(i.tableData[ev.Iface], ev.Event)
		if len(i.tableData[ev.Iface]) > 10 {
			i.tableData[ev.Iface] = i.tableData[ev.Iface][len(i.tableData[ev.Iface])-10:]
		}

		i.app.QueueUpdateDraw(i.updateTable)
		i.mapLock.Unlock()
	}
}

func (i *ifaceTablePrinter) updateTable() {
	i.table.Clear()

	// Table Header
	headers := []string{"Iface", "Protocol", "Direction", "Source", "Src Port", "Destination", "Dst Port", "Flags"}
	for j, h := range headers {
		i.table.SetCell(0, j, tview.NewTableCell(fmt.Sprintf("[::b]%s", h)).
			SetTextColor(tcell.ColorLightCyan).
			SetAlign(tview.AlignCenter).
			SetSelectable(false))
	}

	// Fill rows
	row := 1
	for iface, events := range i.tableData {
		for _, e := range events {
			dir := "Ingress"
			if e.Direction == 1 {
				dir = "Egress"
			}
			proto := protocolName(e.Protocol)
			src := intToIP(e.SrcIP).String()
			dst := intToIP(e.DstIP).String()
			flags := tcpFlagsToString(e.TcpFlags)

			i.table.SetCell(row, 0, tview.NewTableCell(fmt.Sprintf("[white::b]%s", iface)))
			i.table.SetCell(row, 1, tview.NewTableCell(proto))
			i.table.SetCell(row, 2, tview.NewTableCell(dir))
			i.table.SetCell(row, 3, tview.NewTableCell(src))
			i.table.SetCell(row, 4, tview.NewTableCell(fmt.Sprintf("%d", e.SrcPort)))
			i.table.SetCell(row, 5, tview.NewTableCell(dst))
			i.table.SetCell(row, 6, tview.NewTableCell(fmt.Sprintf("%d", e.DstPort)))
			i.table.SetCell(row, 7, tview.NewTableCell(flags))
			row++
		}
	}
}

func protocolName(proto uint8) string {
	switch proto {
	case 6:
		return "TCP"
	case 17:
		return "UDP"
	case 1:
		return "ICMP"
	default:
		return fmt.Sprintf("PROTO(%d)", proto)
	}
}
