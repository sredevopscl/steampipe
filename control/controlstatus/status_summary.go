package controlstatus

type ControlRunStatus uint32

const (
	ControlRunReady ControlRunStatus = 1 << iota
	ControlRunStarted
	ControlRunComplete
	ControlRunError
)

// StatusSummary is a struct containing the counts of each possible control status
type StatusSummary struct {
	Alarm int `json:"alarm"`
	Ok    int `json:"ok"`
	Info  int `json:"info"`
	Skip  int `json:"skip"`
	Error int `json:"error"`
}

func (s *StatusSummary) PassedCount() int {
	return s.Ok + s.Info
}

func (s *StatusSummary) FailedCount() int {
	return s.Alarm + s.Error
}

func (s *StatusSummary) TotalCount() int {
	return s.Alarm + s.Ok + s.Info + s.Skip + s.Error
}

func (s *StatusSummary) Merge(summary *StatusSummary) {
	s.Alarm += summary.Alarm
	s.Ok += summary.Ok
	s.Info += summary.Info
	s.Skip += summary.Skip
	s.Error += summary.Error
}
