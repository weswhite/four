package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
)

func main() {
	w, h, _ := term.GetSize(int(os.Stdout.Fd()))
	if w < 10 {
		w = 80
	}
	if h < 5 {
		h = 24
	}

	old, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "raw mode: %v\n", err)
		os.Exit(1)
	}
	defer term.Restore(int(os.Stdin.Fd()), old)

	// Clear screen, hide cursor, home
	fmt.Print("\033[2J\033[?25l\033[H")
	defer fmt.Print("\033[?25h\033[2J\033[H")

	// Render 3 frames
	for frame := 0; frame < 3; frame++ {
		fmt.Print("\033[H") // home

		for row := 0; row < h-1; row++ {
			fmt.Print("\033[2K") // clear line

			if row == 0 {
				content := fmt.Sprintf(" TEST RENDER  w=%d h=%d frame=%d  %s", w, h, frame, time.Now().Format("15:04:05"))
				if len(content) > w {
					content = content[:w]
				}
				fmt.Print(content)
			} else if row == 1 {
				line := " " + strings.Repeat("─", w-2)
				// Truncate by rune count
				runes := []rune(line)
				if len(runes) > w {
					runes = runes[:w]
				}
				fmt.Print(string(runes))
			} else if row >= 3 && row < 30 {
				sym := fmt.Sprintf("SYM%d", row-3)
				line := fmt.Sprintf("  %-6s  %8s  %10s  %10s  %8s", sym, "$123.45", "$12,345.67", "$1,234.56", "+$567.89")
				runes := []rune(line)
				if len(runes) > w {
					runes = runes[:w]
				}
				fmt.Print(string(runes))
			} else if row == h-2 {
				status := fmt.Sprintf("  press q to quit (row %d/%d)", row, h)
				if len(status) > w {
					status = status[:w]
				}
				fmt.Print(status)
			}

			fmt.Print("\r\n") // explicit CR+LF
		}

		fmt.Print("\033[J") // clear below

		// Wait for input or timeout
		buf := make([]byte, 1)
		os.Stdin.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, _ := os.Stdin.Read(buf)
		if n > 0 && buf[0] == 'q' {
			return
		}
	}
}
