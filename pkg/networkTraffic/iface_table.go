package network

import (
	"fmt"
	"sync"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type ifaceTablePrinter struct {
	chEvent   chan PayLoadTc
	tableData map[string][]Event
	mapLock   sync.RWMutex
	maxTables int
	app       *tview.Application
	tableView *tview.Flex
	tables    map[string]*tview.Table
}

func (i *ifaceTablePrinter) InitTable() {
	i.tableData = make(map[string][]Event)
	i.maxTables = 6
	i.app = tview.NewApplication()
	i.tableView = tview.NewFlex().SetDirection(tview.FlexRow)
	i.tables = make(map[string]*tview.Table)

	go i.forward()

	go func() {
		if err := i.app.SetRoot(i.tableView, true).EnableMouse(true).Run(); err != nil {
			panic(err)
		}
	}()
}

func (i *ifaceTablePrinter) forward() {
	for ev := range i.chEvent {
		i.mapLock.Lock()
		if _, exists := i.tableData[ev.Iface]; !exists {
			if len(i.tableData) >= i.maxTables {
				i.mapLock.Unlock()
				continue // Skip if over max
			}
			i.tableData[ev.Iface] = make([]Event, 0)
			i.tables[ev.Iface] = createInterfaceTable(ev.Iface)
			i.tableView.AddItem(i.tables[ev.Iface], 12, 1, false)
		}
		i.tableData[ev.Iface] = append(i.tableData[ev.Iface], ev.Event)
		if len(i.tableData[ev.Iface]) > 10 {
			i.tableData[ev.Iface] = i.tableData[ev.Iface][len(i.tableData[ev.Iface])-10:]
		}
		i.updateTable(ev.Iface)
		i.mapLock.Unlock()
	}
}

func (i *ifaceTablePrinter) updateTable(iface string) {
	tbl := i.tables[iface]
	events := i.tableData[iface]

	tbl.Clear()
	tbl.SetCell(0, 0, tview.NewTableCell(fmt.Sprintf("INTERFACE: %s", iface)).SetSelectable(false).SetAlign(tview.AlignCenter).SetTextColor(tcell.ColorGreen).SetExpansion(1).SetAttributes(tcell.AttrBold))
	tbl.SetCell(1, 0, tview.NewTableCell("Protocol"))
	tbl.SetCell(1, 1, tview.NewTableCell("Direction"))
	tbl.SetCell(1, 2, tview.NewTableCell("Source"))
	tbl.SetCell(1, 3, tview.NewTableCell("Src Port"))
	tbl.SetCell(1, 4, tview.NewTableCell("Destination"))
	tbl.SetCell(1, 5, tview.NewTableCell("Dst Port"))
	tbl.SetCell(1, 6, tview.NewTableCell("Flags"))

	for idx, e := range events {
		row := idx + 2
		proto := protocolName(e.Protocol)
		dir := "Ingress"
		if e.Direction == 1 {
			dir = "Egress"
		}
		src := intToIP(e.SrcIP).String()
		dst := intToIP(e.DstIP).String()

		flags := tcpFlagsToString(e.TcpFlags)

		tbl.SetCell(row, 0, tview.NewTableCell(proto))
		tbl.SetCell(row, 1, tview.NewTableCell(dir))
		tbl.SetCell(row, 2, tview.NewTableCell(src))
		tbl.SetCell(row, 3, tview.NewTableCell(fmt.Sprintf("%d", e.SrcPort)))
		tbl.SetCell(row, 4, tview.NewTableCell(dst))
		tbl.SetCell(row, 5, tview.NewTableCell(fmt.Sprintf("%d", e.DstPort)))
		tbl.SetCell(row, 6, tview.NewTableCell(flags))
	}
}

func createInterfaceTable(iface string) *tview.Table {
	tbl := tview.NewTable().SetBorders(true)
	tbl.SetTitle(fmt.Sprintf("Interface: %s", iface)).SetTitleColor(tcell.ColorAqua).SetBorder(true)
	return tbl
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

// You must define:
// - type PayLoadTc with fields: Iface string, Event Event
// - type Event with Protocol, Direction, SrcIP, DstIP, SrcPort, DstPort, TcpFlags
// - func intToIP(uint32) string
// - func tcpFlagsToString(uint8) string
