// tinygo build -target=wasm -no-debug -gc=leaking -scheduler=none -o=cal.wasm cal.go
package main

import (
	"fmt"
	"strings"
	"syscall/js"
	"time"
)

type monthstanza [8]string

var today = time.Now().UTC().Truncate(24 * time.Hour)

// fmtmonth renders a single month into 8 lines.
func fmtmonth(year, month int) monthstanza {
	a, r := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC), monthstanza{}
	header := fmt.Sprintf("%d %s", a.Year(), a.Month())
	r[0] = fmt.Sprintf("%-21s", strings.Repeat(" ", 10-len(header)/2)+header)
	r[1] = "Mo Tu We Th Fr Sa Su "

	t := a
	for row := 2; row < 8; row++ {
		for day := time.Weekday(1); day <= 7; day++ {
			if t.Month() == a.Month() && t.Weekday() == day%7 {
				if t.Equal(today) {
					r[row] += fmt.Sprintf("<span class=cbgSpecial><b>%2d</b></span> ", t.Day())
				} else {
					r[row] += fmt.Sprintf("%2d ", t.Day())
				}
				t = t.AddDate(0, 0, 1)
			} else {
				r[row] += "   "
			}
		}
	}
	return r
}

// renderCalendar renders a calendar for a year.
// If year is 0, then the calendar is rendered partially around the current day.
func renderCalendar(year int) string {
	var r string
	startyear, startq := today.Year(), int(today.Month()-1)/3
	endq := startq + 2
	if year != 0 {
		startyear, startq, endq = year, 0, 4
	}
	for q := startq; q < endq; q++ {
		a, b, c, qs := fmtmonth(startyear, q*3+1), fmtmonth(startyear, q*3+2), fmtmonth(startyear, q*3+3), monthstanza{}
		for i := range qs {
			qs[i] = fmt.Sprintf("%s  %s  %s", a[i], b[i], c[i])
		}
		r += strings.Join(qs[:], "\n") + "\n"
	}
	return r
}

func main() {
	js.Global().Get("eResult").Set("innerHTML", renderCalendar(js.Global().Get("year").Int()))
}
