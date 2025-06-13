package tui

import (
	"fmt"
	"strings"
	"syscall"

	"github.com/common-nighthawk/go-figure"
	"github.com/fatih/color"
	"golang.org/x/term"
)

func GetTerminalWidth() int {
	width, _, err := term.GetSize(int(syscall.Stdout))
	if err != nil {
		return 80
	}
	return width
}

func PrintCentered(text string, colorizer *color.Color) {
	width := GetTerminalWidth()
	lines := strings.Split(text, "\n")

	for _, line := range lines {
		trimmedLine := strings.TrimRight(line, " ")
		trimmedLine = strings.TrimLeft(trimmedLine, " ")

		lineLen := len(trimmedLine)
		if lineLen == 0 {
			fmt.Println()
			continue
		}

		padding := (width - lineLen) / 2
		if padding < 0 {
			padding = 0
		}

		paddedLine := fmt.Sprintf("%s%s", strings.Repeat(" ", padding), trimmedLine)
		colorizer.Println(paddedLine)
	}
}

func PrintHeader() {
	fig := figure.NewFigure("KernelKoala", "", true)
	green := color.New(color.FgHiGreen).Add(color.Bold)
	PrintCentered(fig.String(), green)
}
