package srt

// driftTracer estimates the drift between the clock of the peer and the local clock, the same way
// as libsrt's DriftTracer and CTsbpdTime::addDriftSample do. A sample is taken on every ACKACK.
// After a number of samples, the average drift is applied to the TSBPD time of the received packets.
type driftTracer struct {
	maxSpan  uint  // number of samples to average
	maxDrift int64 // microseconds, the drift above that the time base is shifted

	sum  int64 // microseconds
	span uint

	drift     int64 // microseconds, the average drift on top of the shifted time base
	baseShift int64 // microseconds, the accumulated shift of the time base

	refRTT     int64 // microseconds, the reference RTT for the change of the network delay
	rttSamples uint  // number of samples the reference RTT has been taken from
	refSamples uint  // number of samples to take the reference RTT from
}

// newDriftTracer returns a drift tracer with the values that libsrt uses (TSBPD_DRIFT_MAX_SAMPLES and
// TSBPD_DRIFT_MAX_VALUE).
func newDriftTracer() *driftTracer {
	return &driftTracer{
		maxSpan:    1000,
		maxDrift:   5000,
		refSamples: 10,
	}
}

// addSample adds a drift sample. arrival is the local time when the ACKACK arrived and pktBaseTime the time of its
// timestamp on the local time base, both in microseconds. rtt is the RTT measured with this ACKACK in microseconds.
// It returns true if the drift has been updated.
func (d *driftTracer) addSample(arrival, pktBaseTime, rtt int64) bool {
	// The RTT at the time the time base has been established is the reference. libsrt takes the first RTT,
	// here the minimum of the first samples is taken, because a single sample may be inflated.
	if d.rttSamples < d.refSamples {
		if d.rttSamples == 0 || rtt < d.refRTT {
			d.refRTT = rtt
		}
		d.rttSamples++
	}

	// A change of the network delay is approximated by half of the change of the RTT.
	rttDelta := (rtt - d.refRTT) / 2

	d.sum += arrival - (pktBaseTime + d.baseShift) - rttDelta
	d.span++

	if d.span < d.maxSpan {
		return false
	}

	d.drift = d.sum / int64(d.span)

	d.sum = 0
	d.span = 0

	// If the drift is too large, shift the time base and keep only the rest as drift.
	if d.drift > d.maxDrift {
		d.baseShift += d.maxDrift
		d.drift -= d.maxDrift
	} else if d.drift < -d.maxDrift {
		d.baseShift -= d.maxDrift
		d.drift += d.maxDrift
	}

	return true
}

// correction returns the correction in microseconds that has to be added to the TSBPD time of a packet.
func (d *driftTracer) correction() int64 {
	return d.baseShift + d.drift
}
