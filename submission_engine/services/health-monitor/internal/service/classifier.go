package service

type HealthState string

const (
	HealthUnknown  HealthState = "UNKNOWN"
	HealthReady    HealthState = "READY"
	HealthDegraded HealthState = "DEGRADED"
)

type Classifier struct {
	UnhealthyThreshold int
	HealthyThreshold   int
	LatencySLOMS       float64
	failures           int
	successes          int
	state              HealthState
}

func NewClassifier(unhealthyThreshold, healthyThreshold int, latencySLOMS float64) *Classifier {
	if unhealthyThreshold == 0 {
		unhealthyThreshold = 3
	}
	if healthyThreshold == 0 {
		healthyThreshold = 2
	}
	if latencySLOMS == 0 {
		latencySLOMS = 500
	}
	return &Classifier{
		UnhealthyThreshold: unhealthyThreshold,
		HealthyThreshold:   healthyThreshold,
		LatencySLOMS:       latencySLOMS,
		state:              HealthUnknown,
	}
}

func (c *Classifier) Observe(result ProbeResult) HealthState {
	healthy := result.Healthy && result.LatencyMS <= c.LatencySLOMS
	if healthy {
		c.successes++
		c.failures = 0
		if c.successes >= c.HealthyThreshold {
			c.state = HealthReady
		}
		return c.state
	}
	c.failures++
	c.successes = 0
	if c.failures >= c.UnhealthyThreshold {
		c.state = HealthDegraded
	}
	return c.state
}
