package progresswatch

type Observation uint8

const (
	Progressed Observation = iota
	AtLatest
	Waiting
	Stalled
)

const unchangedChecksBeforeStall = 2

type Detector struct {
	previous        int64
	unchangedChecks int
}

func NewDetector(initialProgress int64) *Detector {
	return &Detector{previous: initialProgress}
}

func (d *Detector) Observe(current, latest int64) Observation {
	if current != d.previous {
		d.previous = current
		d.unchangedChecks = 0
		return Progressed
	}
	if current == latest {
		d.unchangedChecks = 0
		return AtLatest
	}

	d.unchangedChecks++
	if d.unchangedChecks < unchangedChecksBeforeStall {
		return Waiting
	}

	d.unchangedChecks = 0
	return Stalled
}
