package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/datarhei/core/v16/ffmpeg"
	"github.com/datarhei/core/v16/ffmpeg/parse"
	"github.com/datarhei/core/v16/resources"

	"github.com/sergi/go-diff/diffmatchpatch"
)

type fftest struct {
	name    string
	what    string
	init    []string
	test    []string
	cleanup []string
}

func main() {
	abinary := ""
	bbinary := ""

	flag.StringVar(&abinary, "aversion", "", "version A of ffmpeg")
	flag.StringVar(&bbinary, "bversion", "", "version B of ffmpeg")

	flag.Parse()

	if len(abinary) == 0 || len(bbinary) == 0 {
		slog.Error("aversion and bversion must be given")
		os.Exit(1)
	}

	r, err := resources.New(resources.Config{})
	if err != nil {
		slog.Error("failed resources", "error", err)
		os.Exit(1)
	}

	a, err := ffmpeg.New(ffmpeg.Config{
		Binary:   abinary,
		Resource: r,
	})
	if err != nil {
		slog.Error("failed ffmpeg binary", "error", err)
		os.Exit(1)
	}

	b, err := ffmpeg.New(ffmpeg.Config{
		Binary:   bbinary,
		Resource: r,
	})
	if err != nil {
		slog.Error("failed ffmpeg binary", "error", err)
		os.Exit(1)
	}

	tests := []fftest{
		{
			name: "version",
			test: []string{
				"-i",
			},
			what: "prelude",
		},
		{
			name: "final duration with vframes",
			init: []string{
				"-t", "30", "-f", "lavfi", "-i", "testsrc2", "-codec:v", "libx264", "-preset", "ultrafast", "-y", "file.mp4",
			},
			test: []string{
				"-i", "file.mp4", "-codec:v", "copy", "-f", "null", "-", "-vframes", "1", "-f", "null", "-",
			},
			cleanup: []string{"file.mp4"},
			what:    "progress",
		},
	}

	dmp := diffmatchpatch.New()

	for _, test := range tests {
		fmt.Printf("---- %s ----\n", test.name)
		pa, err := run_test(a, test)
		if err != nil {
			fmt.Printf("test %s with %s failed: %v\n", test.name, abinary, err)
			continue
		}
		pb, err := run_test(b, test)
		if err != nil {
			fmt.Printf("test %s with %s failed: %v\n", test.name, bbinary, err)
			continue
		}

		jpa, _ := json.MarshalIndent(pa, "", "   ")
		jpb, _ := json.MarshalIndent(pb, "", "   ")

		diffs := dmp.DiffMain(string(jpa), string(jpb), false)

		fmt.Println(dmp.DiffPrettyText(diffs))
	}
}

func run_test(ff ffmpeg.FFmpeg, test fftest) (any, error) {
	defer func() {
		for _, file := range test.cleanup {
			os.Remove(file)
		}
	}()

	parser := parse.New(parse.Config{
		LogHistory: 1,
	})

	wg := sync.WaitGroup{}

	if len(test.init) != 0 {
		wg.Add(1)

		p, err := ff.New(ffmpeg.ProcessConfig{
			Args:   test.init,
			Parser: parse.New(parse.Config{}),
			OnExit: func(string) {
				wg.Done()
			},
		})
		if err != nil {
			return parse.Progress{}, err
		}

		p.Start()

		wg.Wait()

		status := p.Status()
		if status.States.Finished != 1 {
			return parse.Progress{}, fmt.Errorf("init failed: %s", parser.LastLogline())
		}
	}

	wg.Add(1)

	p, err := ff.New(ffmpeg.ProcessConfig{
		Args:   test.test,
		Parser: parser,
		OnExit: func(string) {
			wg.Done()
		},
	})
	if err != nil {
		return parse.Progress{}, err
	}

	p.Start()

	wg.Wait()

	status := p.Status()
	if status.States.Finished != 1 {
		return parse.Progress{}, fmt.Errorf("test failed: %s", parser.LastLogline())
	}

	report := parser.ReportHistory()
	if len(report) < 1 {
		return parse.Progress{}, fmt.Errorf("no report available")
	}

	if test.what == "progress" {
		return report[len(report)-1].Progress, nil
	} else if test.what == "prelude" {
		return report[len(report)-1].Prelude, nil
	}

	return parse.Progress{}, fmt.Errorf("no proper what provided")
}
