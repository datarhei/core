package probe

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/process"
	"github.com/datarhei/core/v16/restream/app"
)

type Parser interface {
	process.Parser

	Probe() app.Probe
}

type Config struct {
	Logger log.Logger
}

type prober struct {
	data   []process.Line
	inputs []probeIO

	logger log.Logger
}

func New(config Config) Parser {
	p := &prober{
		logger: config.Logger,
	}

	if p.logger == nil {
		p.logger = log.New("Parser")
	}

	return p
}

func (p *prober) Probe() app.Probe {
	probe := app.Probe{}

	for _, io := range p.inputs {
		probe.Streams = append(probe.Streams, io.export())
	}

	probe.Log = make([]string, len(p.data))
	for i, line := range p.data {
		probe.Log[i] = line.Data
	}

	return probe
}

func (p *prober) Parse(line string) uint64 {
	if strings.HasPrefix(line, "avstream.progress:") {
		return 0
	}

	p.data = append(p.data, process.Line{
		Timestamp: time.Now(),
		Data:      line,
	})

	return 0
}

func (p *prober) parseJSON(line string) {
	inputs := []probeIO{}

	err := json.Unmarshal([]byte(line), &inputs)
	if err != nil {
		p.logger.WithFields(log.Fields{
			"line":  line,
			"error": err,
		}).Error().Log("Failed parsing inputs")
	}

	p.inputs = inputs
}

func (p *prober) parseDefault() {
	// Input #0, lavfi, from 'testsrc=size=1280x720:rate=25':
	// Input #1, lavfi, from 'anullsrc=r=44100:cl=stereo':
	// Output #0, hls, to './data/testsrc.m3u8':
	reFormat := regexp.MustCompile(`^Input #([0-9]+), (.*?), (from|to) '([^']+)`)

	// Duration: 00:01:02.28, start: 0.000000, bitrate: 5895 kb/s
	// Duration: N/A, start: 0.000000, bitrate: 5895 kb/s
	reDuration := regexp.MustCompile(`Duration: ([0-9]+):([0-9]+):([0-9]+)\.([0-9]+)`)

	// Stream #0:0: Video: rawvideo (RGB[24] / 0x18424752), rgb24, 1280x720 [SAR 1:1 DAR 16:9], 25 tbr, 25 tbn, 25 tbc
	// Stream #1:0: Audio: pcm_u8, 44100 Hz, stereo, u8, 705 kb/s
	// Stream #0:0: Video: h264 (libx264), yuv420p(progressive), 1280x720 [SAR 1:1 DAR 16:9], q=-1--1, 25 fps, 90k tbn, 25 tbc
	// Stream #0:1: Audio: aac (LC), 44100 Hz, stereo, fltp, 64 kb/s
	reStream := regexp.MustCompile(`Stream #([0-9]+):([0-9]+)(?:\(([a-z]+)\))?: (Video|Audio|Subtitle): (.*)`)
	reStreamCodec := regexp.MustCompile(`^([^\s,]+)`)
	reStreamVideoPixfmtSize := regexp.MustCompile(`, ([0-9A-Za-z]+)(\([^\)]+\))?, ([0-9]+)x([0-9]+)`)
	reStreamVideoFPS := regexp.MustCompile(`, ([0-9]+(\.[0-9]+)?) fps`)
	reStreamAudio := regexp.MustCompile(`, ([0-9]+) Hz, ([^,]+)`)
	reStreamBitrate := regexp.MustCompile(`, ([0-9]+) kb/s`)

	format := ""
	address := ""
	var duration float64 = 0.0

	for _, line := range p.data {
		if matches := reFormat.FindStringSubmatch(line.Data); matches != nil {
			format = matches[2]
			address = matches[4]

			continue
		}

		if matches := reDuration.FindStringSubmatch(line.Data); matches != nil {
			duration = 0.0

			// hours
			if x, err := strconv.ParseFloat(matches[1], 64); err == nil {
				duration += x * 60 * 60
			}

			// minutes
			if x, err := strconv.ParseFloat(matches[2], 64); err == nil {
				duration += x * 60
			}

			// seconds
			if x, err := strconv.ParseFloat(matches[3], 64); err == nil {
				duration += x
			}

			// fractions
			if x, err := strconv.ParseFloat(matches[4], 64); err == nil {
				duration += x / 100
			}

			continue
		}

		if matches := reStream.FindStringSubmatch(line.Data); matches != nil {
			io := probeIO{}

			io.Address = address
			io.Format = format
			io.Duration = duration

			if x, err := strconv.ParseUint(matches[1], 10, 64); err == nil {
				io.Index = x
			}

			if x, err := strconv.ParseUint(matches[2], 10, 64); err == nil {
				io.Stream = x
			}

			io.Language = "und"
			if len(matches[3]) == 3 {
				io.Language = matches[3]
			}

			io.Type = strings.ToLower(matches[4])

			streamDetail := matches[5]

			if matches = reStreamCodec.FindStringSubmatch(streamDetail); matches != nil {
				io.Codec = matches[1]
			}

			if matches = reStreamBitrate.FindStringSubmatch(streamDetail); matches != nil {
				if x, err := strconv.ParseFloat(matches[1], 64); err == nil {
					io.Bitrate = x
				}
			}

			if io.Type == "video" {
				if matches = reStreamVideoPixfmtSize.FindStringSubmatch(streamDetail); matches != nil {
					io.Pixfmt = matches[1]

					if x, err := strconv.ParseUint(matches[3], 10, 64); err == nil {
						io.Width = x
					}
					if x, err := strconv.ParseUint(matches[4], 10, 64); err == nil {
						io.Height = x
					}
				}

				if matches = reStreamVideoFPS.FindStringSubmatch(streamDetail); matches != nil {
					if x, err := strconv.ParseFloat(matches[1], 64); err == nil {
						io.FPS = x
					}
				}

			} else if io.Type == "audio" {
				if matches = reStreamAudio.FindStringSubmatch(streamDetail); matches != nil {
					if x, err := strconv.ParseUint(matches[1], 10, 64); err == nil {
						io.Sampling = x
					}

					io.Layout = matches[2]
				}
			}

			p.inputs = append(p.inputs, io)
		}
	}
}

func (p *prober) Log() []process.Line {
	return p.data
}

func (p *prober) ResetStats() {
	hasJSON := false
	prefix := "ffmpeg.inputs:"

	for _, line := range p.data {
		isFFmpegInputs := strings.HasPrefix(line.Data, prefix)

		if isFFmpegInputs {
			p.parseJSON(strings.TrimPrefix(line.Data, prefix))
			hasJSON = true

			break
		}
	}

	if !hasJSON {
		p.parseDefault()
	}
}

func (p *prober) ResetLog() {
	p.data = []process.Line{}
	p.inputs = []probeIO{}
}
