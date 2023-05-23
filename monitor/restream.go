package monitor

import (
	"strconv"

	"github.com/datarhei/core/v16/monitor/metric"
	"github.com/datarhei/core/v16/restream"
)

type restreamCollector struct {
	prefix                     string
	r                          restream.Restreamer
	restreamProcessDescr       *metric.Description
	restreamProcessStatesDescr *metric.Description
	restreamProcessIODescr     *metric.Description
	restreamStatesDescr        *metric.Description
}

func NewRestreamCollector(r restream.Restreamer) metric.Collector {
	c := &restreamCollector{
		prefix: "restream",
		r:      r,
	}

	c.restreamProcessDescr = metric.NewDesc("restream_process", "Current process values by name", []string{"processid", "state", "order", "name"})
	c.restreamProcessStatesDescr = metric.NewDesc("restream_process_states", "Current process state", []string{"processid", "state"})
	c.restreamProcessIODescr = metric.NewDesc("restream_io", "Current process IO values by name", []string{"processid", "type", "id", "address", "index", "stream", "media", "name"})
	c.restreamStatesDescr = metric.NewDesc("restream_state", "Summarized current process states", []string{"state"})

	return c
}

func (c *restreamCollector) Prefix() string {
	return c.prefix
}

func (c *restreamCollector) Describe() []*metric.Description {
	return []*metric.Description{
		c.restreamProcessDescr,
		c.restreamProcessStatesDescr,
		c.restreamProcessIODescr,
		c.restreamStatesDescr,
	}
}

func (c *restreamCollector) Collect() metric.Metrics {
	metrics := metric.NewMetrics()

	value := float64(0)

	states := map[string]float64{
		"failed":    0,
		"finished":  0,
		"finishing": 0,
		"killed":    0,
		"running":   0,
		"starting":  0,
	}

	ids := c.r.GetProcessIDs("", "", "", "")

	for _, id := range ids {
		state, _ := c.r.GetProcessState(id)
		if state == nil {
			continue
		}

		proc, _ := c.r.GetProcess(id)
		if proc == nil {
			continue
		}

		states[state.State]++

		metrics.Add(metric.NewValue(c.restreamProcessDescr, float64(state.Progress.Frame), id.String(), state.State, state.Order, "frame"))
		metrics.Add(metric.NewValue(c.restreamProcessDescr, float64(state.Progress.FPS), id.String(), state.State, state.Order, "fps"))
		metrics.Add(metric.NewValue(c.restreamProcessDescr, float64(state.Progress.Speed), id.String(), state.State, state.Order, "speed"))
		metrics.Add(metric.NewValue(c.restreamProcessDescr, state.Progress.Quantizer, id.String(), state.State, state.Order, "q"))
		metrics.Add(metric.NewValue(c.restreamProcessDescr, float64(state.Progress.Size), id.String(), state.State, state.Order, "size"))
		metrics.Add(metric.NewValue(c.restreamProcessDescr, state.Progress.Time, id.String(), state.State, state.Order, "time"))
		metrics.Add(metric.NewValue(c.restreamProcessDescr, float64(state.Progress.Drop), id.String(), state.State, state.Order, "drop"))
		metrics.Add(metric.NewValue(c.restreamProcessDescr, float64(state.Progress.Dup), id.String(), state.State, state.Order, "dup"))
		metrics.Add(metric.NewValue(c.restreamProcessDescr, float64(state.Progress.Packet), id.String(), state.State, state.Order, "packet"))
		metrics.Add(metric.NewValue(c.restreamProcessDescr, state.Progress.Bitrate, id.String(), state.State, state.Order, "bitrate"))
		metrics.Add(metric.NewValue(c.restreamProcessDescr, state.CPU, id.String(), state.State, state.Order, "cpu"))
		metrics.Add(metric.NewValue(c.restreamProcessDescr, float64(state.Memory), id.String(), state.State, state.Order, "memory"))
		metrics.Add(metric.NewValue(c.restreamProcessDescr, state.Duration, id.String(), state.State, state.Order, "uptime"))

		if proc.Config != nil {
			metrics.Add(metric.NewValue(c.restreamProcessDescr, proc.Config.LimitCPU, id.String(), state.State, state.Order, "cpu_limit"))
			metrics.Add(metric.NewValue(c.restreamProcessDescr, float64(proc.Config.LimitMemory), id.String(), state.State, state.Order, "memory_limit"))
		}

		metrics.Add(metric.NewValue(c.restreamProcessStatesDescr, float64(state.States.Failed), id.String(), "failed"))
		metrics.Add(metric.NewValue(c.restreamProcessStatesDescr, float64(state.States.Finished), id.String(), "finished"))
		metrics.Add(metric.NewValue(c.restreamProcessStatesDescr, float64(state.States.Finishing), id.String(), "finishing"))
		metrics.Add(metric.NewValue(c.restreamProcessStatesDescr, float64(state.States.Killed), id.String(), "killed"))
		metrics.Add(metric.NewValue(c.restreamProcessStatesDescr, float64(state.States.Running), id.String(), "running"))
		metrics.Add(metric.NewValue(c.restreamProcessStatesDescr, float64(state.States.Starting), id.String(), "starting"))

		for i := range state.Progress.Input {
			io := &state.Progress.Input[i]

			index := strconv.FormatUint(io.Index, 10)
			stream := strconv.FormatUint(io.Stream, 10)

			metrics.Add(metric.NewValue(c.restreamProcessIODescr, float64(io.Frame), id.String(), "input", io.ID, io.Address, index, stream, io.Type, "frame"))
			metrics.Add(metric.NewValue(c.restreamProcessIODescr, float64(io.FPS), id.String(), "input", io.ID, io.Address, index, stream, io.Type, "fps"))
			metrics.Add(metric.NewValue(c.restreamProcessIODescr, float64(io.Packet), id.String(), "input", io.ID, io.Address, index, stream, io.Type, "packet"))
			metrics.Add(metric.NewValue(c.restreamProcessIODescr, float64(io.PPS), id.String(), "input", io.ID, io.Address, index, stream, io.Type, "pps"))
			metrics.Add(metric.NewValue(c.restreamProcessIODescr, float64(io.Size), id.String(), "input", io.ID, io.Address, index, stream, io.Type, "size"))
			metrics.Add(metric.NewValue(c.restreamProcessIODescr, float64(io.Bitrate), id.String(), "input", io.ID, io.Address, index, stream, io.Type, "bitrate"))

			if io.AVstream != nil {
				a := io.AVstream

				metrics.Add(metric.NewValue(c.restreamProcessIODescr, float64(a.Queue), id.String(), "input", io.ID, io.Address, index, stream, io.Type, "avstream_queue"))
				metrics.Add(metric.NewValue(c.restreamProcessIODescr, float64(a.Dup), id.String(), "input", io.ID, io.Address, index, stream, io.Type, "avstream_dup"))
				metrics.Add(metric.NewValue(c.restreamProcessIODescr, float64(a.Drop), id.String(), "input", io.ID, io.Address, index, stream, io.Type, "avstream_drop"))
				metrics.Add(metric.NewValue(c.restreamProcessIODescr, float64(a.Enc), id.String(), "input", io.ID, io.Address, index, stream, io.Type, "avstream_enc"))

				value = 0
				if a.Looping {
					value = 1
				}
				metrics.Add(metric.NewValue(c.restreamProcessIODescr, value, id.String(), "input", io.ID, io.Address, index, stream, io.Type, "avstream_looping"))

				value = 0
				if a.Duplicating {
					value = 1
				}
				metrics.Add(metric.NewValue(c.restreamProcessIODescr, value, id.String(), "input", io.ID, io.Address, index, stream, io.Type, "avstream_duplicating"))
			}
		}

		for i := range state.Progress.Output {
			io := &state.Progress.Output[i]

			index := strconv.FormatUint(io.Index, 10)
			stream := strconv.FormatUint(io.Stream, 10)

			metrics.Add(metric.NewValue(c.restreamProcessIODescr, float64(io.Frame), id.String(), "output", io.ID, io.Address, index, stream, io.Type, "frame"))
			metrics.Add(metric.NewValue(c.restreamProcessIODescr, float64(io.FPS), id.String(), "output", io.ID, io.Address, index, stream, io.Type, "fps"))
			metrics.Add(metric.NewValue(c.restreamProcessIODescr, float64(io.Packet), id.String(), "output", io.ID, io.Address, index, stream, io.Type, "packet"))
			metrics.Add(metric.NewValue(c.restreamProcessIODescr, float64(io.PPS), id.String(), "output", io.ID, io.Address, index, stream, io.Type, "pps"))
			metrics.Add(metric.NewValue(c.restreamProcessIODescr, float64(io.Size), id.String(), "output", io.ID, io.Address, index, stream, io.Type, "size"))
			metrics.Add(metric.NewValue(c.restreamProcessIODescr, float64(io.Bitrate), id.String(), "output", io.ID, io.Address, index, stream, io.Type, "bitrate"))
			metrics.Add(metric.NewValue(c.restreamProcessIODescr, float64(io.Quantizer), id.String(), "output", io.ID, io.Address, index, stream, io.Type, "q"))
		}
	}

	for state, value := range states {
		metrics.Add(metric.NewValue(c.restreamStatesDescr, value, state))
	}

	return metrics
}

func (c *restreamCollector) Stop() {}
