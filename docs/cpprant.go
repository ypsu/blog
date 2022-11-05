package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"net/url"
	"os"
	"os/exec"
	"strings"

	_ "embed"
)

//go:embed cpprant.script
var script string

type wavFormat struct {
	audioFormat   int16
	numChannels   int16
	sampleRate    int32
	byteRate      int32
	blockAlign    int16
	bitsPerSample int16
}

var expectedFormat = wavFormat{
	audioFormat:   1,
	numChannels:   1,
	sampleRate:    24000,
	byteRate:      48000,
	blockAlign:    2,
	bitsPerSample: 16,
}

type event struct {
	// if text is empty, then this is a pause event.
	actor       string
	text        string
	sampleBytes []byte

	// if character is non-zero, this is an edit operation.
	// if character is 8, it's a delete operation.
	editpos   int
	character rune
}

var zeroBytes = make([]byte, 5*expectedFormat.byteRate)

func numRead(r io.Reader, data any) {
	if err := binary.Read(r, binary.LittleEndian, data); err != nil {
		log.Fatalf("read failed: %v", err)
	}
}

func main() {
	// check that the voice synthesis software and the voices are present.
	if _, err := os.Stat("/usr/bin/ffmpeg"); err != nil {
		log.Fatal("missing software, run `sudo apt install ffmpeg`.")
	}
	if _, err := os.Stat("/usr/bin/RHVoice-test"); err != nil {
		log.Fatal("missing software, run `sudo apt install rhvoice`.")
	}
	if _, err := os.Stat("/usr/share/RHVoice/voices/slt"); err != nil {
		log.Fatal("missing slt voice, run `sudo apt install rhvoice-english`.")
	}
	if _, err := os.Stat("/usr/share/RHVoice/voices/bdl"); err != nil {
		log.Fatal("missing bdl voice, run `sudo apt install rhvoice-english`.")
	}

	// initialization.
	var events []event
	os.Mkdir("/tmp/cpprant/", 0700)
	hasher := fnv.New64()
	// start with a short pause.
	events = append(events, event{sampleBytes: zeroBytes[:expectedFormat.byteRate/1000]})

	// process the script.
	totalSamples := 0
	for _, line := range strings.Split(script, "\n") {
		if line == "" || line[0] == '#' {
			continue
		}
		r := strings.NewReader(line)
		var directive string
		fmt.Fscan(r, &directive)

		if directive == "pause" {
			// this pause directive that generates `dur` ms long silence.
			var dur int32
			fmt.Fscan(r, &dur)
			sb := zeroBytes[:expectedFormat.byteRate*dur/1000]
			events = append(events, event{sampleBytes: sb})
			totalSamples += len(sb) / (int(expectedFormat.bitsPerSample) / 8)
			continue
		}

		if directive == "edit" {
			// this edit directive is in the form of "duration pos del newstr"
			var dur, pos, del int
			var str string
			fmt.Fscanf(r, "%d %d %d %q", &dur, &pos, &del, &str)
			runes := []rune(str)
			dur /= del + len(runes)
			for i := 0; i < del; i++ {
				sb := zeroBytes[:int(expectedFormat.byteRate)*dur/1000]
				events = append(events, event{
					sampleBytes: sb,
					editpos:     pos,
					character:   8,
				})
				totalSamples += len(sb) / (int(expectedFormat.bitsPerSample) / 8)
			}
			for i, ch := range runes {
				sb := zeroBytes[:int(expectedFormat.byteRate)*dur/1000]
				events = append(events, event{
					sampleBytes: sb,
					editpos:     pos + i,
					character:   ch,
				})
				totalSamples += len(sb) / (int(expectedFormat.bitsPerSample) / 8)
			}
			continue
		}

		// this is a speech event that needs voice synthesis.
		person := directive
		var text string
		fmt.Fscanf(r, "%q", &text)
		readtext := strings.ReplaceAll(text, "c++", "c plus plus")
		voice := "slt"
		if person == "dog" {
			voice = "bdl"
		}
		hasher.Reset()
		hasher.Write([]byte(voice))
		hasher.Write([]byte(readtext))
		filename := fmt.Sprintf("/tmp/cpprant/%x.wav", hasher.Sum64())
		if _, err := os.Stat(filename); err != nil {
			cmd := exec.Command("/usr/bin/RHVoice-test", "-p", voice, "-o", filename)
			cmd.Stdin = strings.NewReader(readtext)
			if err := cmd.Run(); err != nil {
				log.Fatal(err)
			}
		}
		data, err := os.ReadFile(filename)
		if err != nil {
			log.Fatal(err)
		}
		wavr := bytes.NewReader(data)
		var str [4]byte
		if n, err := wavr.Read(str[:]); err != nil || n != 4 || string(str[:]) != "RIFF" {
			log.Fatal("RIFF read fail: %v %v.", n, err)
		}
		if n, err := wavr.Read(str[:]); err != nil || n != 4 {
			log.Fatalf("total size read fail: %v %v.", n, err)
		}
		if n, err := wavr.Read(str[:]); err != nil || n != 4 || string(str[:]) != "WAVE" {
			log.Fatalf("WAVE read fail: %v %v.", n, err)
		}
		if n, err := wavr.Read(str[:]); err != nil || n != 4 || string(str[:]) != "fmt " {
			log.Fatalf("fmt read fail: %v %v.", n, err)
		}
		if n, err := wavr.Read(str[:]); err != nil || n != 4 || str != [4]byte{16, 0, 0, 0} {
			log.Fatalf("16 read fail: %v %v.", n, err)
		}
		var wavfmt wavFormat
		numRead(wavr, &wavfmt.audioFormat)
		numRead(wavr, &wavfmt.numChannels)
		numRead(wavr, &wavfmt.sampleRate)
		numRead(wavr, &wavfmt.byteRate)
		numRead(wavr, &wavfmt.blockAlign)
		numRead(wavr, &wavfmt.bitsPerSample)
		if wavfmt != expectedFormat {
			log.Fatalf("unexpected format %+v, want: %+v", wavfmt, expectedFormat)
		}
		if n, err := wavr.Read(str[:]); err != nil || n != 4 || string(str[:]) != "data" {
			log.Fatalf("data read fail: %v %v.", n, err)
		}
		var datasize int32
		numRead(wavr, &datasize)
		sampleBytes := make([]byte, datasize)
		if n, err := wavr.Read(sampleBytes); err != nil || n != int(datasize) {
			log.Fatalf("data loading fail: %v %v.", n, err)
		}
		if wavr.Len() != 0 {
			log.Fatal("unread content.")
		}
		duration := len(sampleBytes) / (int(wavfmt.byteRate) / 1000)
		totalSamples += len(sampleBytes) / (int(wavfmt.bitsPerSample) / 8)
		log.Printf("generated %d ms audio for %q.", duration, text)
		events = append(events, event{
			actor:       person,
			text:        text,
			sampleBytes: sampleBytes,
		})
		// add a short pause after each sentence.
		sb := zeroBytes[:expectedFormat.byteRate/4]
		events = append(events, event{sampleBytes: sb})
		totalSamples += len(sb) / (int(expectedFormat.bitsPerSample) / 8)
	}

	// write the complete audio.
	totalDuration := totalSamples / (int(expectedFormat.sampleRate) / 1000)
	log.Printf("total audio duration: %d s.", totalDuration/1000)
	log.Print("writing the resulting audio.")
	buf := &bytes.Buffer{}
	buf.WriteString("RIFF")
	binary.Write(buf, binary.LittleEndian, int32(totalSamples*int(expectedFormat.bitsPerSample)/8+36))
	buf.WriteString("WAVEfmt ")
	binary.Write(buf, binary.LittleEndian, int32(16))
	binary.Write(buf, binary.LittleEndian, expectedFormat.audioFormat)
	binary.Write(buf, binary.LittleEndian, expectedFormat.numChannels)
	binary.Write(buf, binary.LittleEndian, expectedFormat.sampleRate)
	binary.Write(buf, binary.LittleEndian, expectedFormat.byteRate)
	binary.Write(buf, binary.LittleEndian, expectedFormat.blockAlign)
	binary.Write(buf, binary.LittleEndian, expectedFormat.bitsPerSample)
	buf.WriteString("data")
	binary.Write(buf, binary.LittleEndian, int32(totalSamples*int(expectedFormat.bitsPerSample)/8))
	for _, e := range events {
		if _, err := buf.Write(e.sampleBytes); err != nil {
			log.Fatal(err)
		}
	}
	if err := os.WriteFile("/tmp/cpprant/cpprant.wav", buf.Bytes(), 0600); err != nil {
		log.Fatal(err)
	}

	// convert to ogg.
	log.Print("converting to .ogg.")
	cmd := exec.Command("ffmpeg", "-y", "-i", "/tmp/cpprant/cpprant.wav", "-q", "0", "cpprant.ogg")
	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}

	// compute the dialog timestamps.
	log.Print("writing the resulting datafile.")
	dataw := &strings.Builder{}
	fmt.Fprint(dataw, "let rawdata = `\n")
	sampleBytes, totalDur := 0, 0
	for _, e := range events {
		if e.character == 8 {
			dur := int(float64(len(e.sampleBytes)) / float64(expectedFormat.byteRate) * 1000.0)
			fmt.Fprintf(dataw, "%d del %d\n", dur, e.editpos)
			totalDur += dur
		} else if e.character != 0 {
			dur := int(float64(len(e.sampleBytes)) / float64(expectedFormat.byteRate) * 1000.0)
			fmt.Fprintf(dataw, "%d add %d %s\n", dur, e.editpos, url.PathEscape(string(e.character)))
			totalDur += dur
		} else if len(e.text) == 0 {
			dur := int(float64(len(e.sampleBytes)) / float64(expectedFormat.byteRate) * 1000.0)
			fmt.Fprintf(dataw, "%d break\n", dur)
			totalDur += dur
		}
		chars := 0
		for _, w := range strings.Split(e.text, " ") {
			if len(e.text) == 0 {
				continue
			}
			percent := float64(chars+len(w)+1) / float64(len(e.text))
			offset := ((float64(sampleBytes) + percent*float64(len(e.sampleBytes))) / float64(expectedFormat.byteRate)) * 1000.0
			dur := int(offset) - totalDur
			fmt.Fprintf(dataw, "%d %s %s\n", dur, e.actor, w)
			totalDur += dur
			chars += len(w) + 1
		}
		sampleBytes += len(e.sampleBytes)
	}
	data := dataw.String() + "`\n"
	if err := os.WriteFile("cpprant.data", []byte(data), 0600); err != nil {
		log.Fatal(err)
	}
}
