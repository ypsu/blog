#!/usr/bin/gnuplot

set terminal webp size 1280,720 font "Noto Sans"
set output "mementomori.webp"
set format x "%Y-%b"
set ylabel "size (KB)"
set key below
set xdata time
set timefmt "%Y-%m-%d"
plot "mementomori.data" using 1:2 with lines smooth acsplines lw 5 title ""
