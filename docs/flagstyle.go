package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/itzg/go-flagsfiller"
)

var subcommand string

var globalFlags struct {
	Force   bool `usage:"auto-confirm all confirmation prompts. dangerous."`
	Verbose bool `usage:"print debug information."`
}

func globalUsage() {
	fmt.Fprintf(flag.CommandLine.Output(), "usage of compressor: compressor [flags...] [subcommand]\n\n")
	fmt.Fprintf(flag.CommandLine.Output(), "subcommands:\n\n")
	fmt.Fprintf(flag.CommandLine.Output(), "  compress:   compress a file.\n")
	fmt.Fprintf(flag.CommandLine.Output(), "  decompress: decompress a file.\n")
	fmt.Fprintf(flag.CommandLine.Output(), "\nuse `compressor -help [subcommand]` to get more help.\n")
}

var compressFlags struct {
	Input  string `usage:"input filename." default:"/dev/stdin"`
	Output string `usage:"output filename." default:"/dev/stdout"`
	Level  int    `usage:"compression level between 1 and 9, 9 the best but slowest." default:"5"`
}

func compressUsage() {
	global, subcommand := flag.NewFlagSet("global", flag.ContinueOnError), flag.NewFlagSet("subcommand", flag.ContinueOnError)
	flagsfiller.New().Fill(global, &globalFlags)
	flagsfiller.New().Fill(subcommand, &compressFlags)

	fmt.Fprintf(flag.CommandLine.Output(), "usage of the compress subcommand: compressor [flags...] compress\n\n")
	fmt.Fprintf(flag.CommandLine.Output(), "compresses a file.\n\n")
	fmt.Fprintf(flag.CommandLine.Output(), "subcommand flags:\n")
	subcommand.PrintDefaults()
	fmt.Fprintf(flag.CommandLine.Output(), "\nglobal flags:\n")
	global.PrintDefaults()
}

var decompressFlags struct {
	Input            string `usage:"input filename." default:"/dev/stdin"`
	Output           string `usage:"output filename." default:"/dev/stdout"`
	AllowPassthrough bool   `usage:"don't error out if input is not compressed. just copy the input to output in that case."`
}

func decompressUsage() {
	global, subcommand := flag.NewFlagSet("global", flag.ContinueOnError), flag.NewFlagSet("subcommand", flag.ContinueOnError)
	flagsfiller.New().Fill(global, &globalFlags)
	flagsfiller.New().Fill(subcommand, &compressFlags)

	fmt.Fprintf(flag.CommandLine.Output(), "usage of the decompress subcommand: compressor [flags...] decompress\n\n")
	fmt.Fprintf(flag.CommandLine.Output(), "decompresses a file.\n\n")
	fmt.Fprintf(flag.CommandLine.Output(), "subcommand flags:\n")
	subcommand.PrintDefaults()
	fmt.Fprintf(flag.CommandLine.Output(), "\nglobal flags:\n")
	global.PrintDefaults()
}

func run() error {
	flagsfiller.New().Fill(flag.CommandLine, &globalFlags)
	flag.Usage = globalUsage

	// extract the subcommand.
	for _, arg := range os.Args[1:] {
		if arg == "--" {
			break
		}
		if strings.HasPrefix(arg, "-") {
			if subcommand != "" {
				return fmt.Errorf("main.BadFlagOrder arg=%s (all flags must come before the subcommand and must have the -flag=value form)", arg)
			}
			continue
		}
		if subcommand == "" {
			subcommand = arg
			if arg == "" {
				return fmt.Errorf("main.EmptySubcommand")
			}
		}
	}

	// Parse the flags with the subcommand flags added.
	switch subcommand {
	case "compress":
		flagsfiller.New().Fill(flag.CommandLine, &compressFlags)
		flag.CommandLine.Usage = compressUsage
	case "decompress":
		flagsfiller.New().Fill(flag.CommandLine, &decompressFlags)
		flag.CommandLine.Usage = decompressUsage
	case "":
		globalUsage()
		return nil
	default:
		return fmt.Errorf("main.UnknownSubcommand subcommand=%s", subcommand)
	}
	flag.Parse()

	if subcommand == "compress" {
		fmt.Printf("compressing %s into %s, level=%d, verbose=%t.\n", compressFlags.Input, compressFlags.Output, compressFlags.Level, globalFlags.Verbose)
	} else if subcommand == "decompress" {
		fmt.Printf("decompressing %s into %s, allowPassthrough=T, verbose=%t.\n", decompressFlags.Input, decompressFlags.Output, globalFlags.Verbose, decompressFlags.AllowPassthrough)
	} else {
		return fmt.Errorf("main.UnknownSubcommand subcommand=%s", subcommand)
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
