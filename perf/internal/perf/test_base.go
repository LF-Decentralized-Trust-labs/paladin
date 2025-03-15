package perf

import (
	"github.com/go-resty/resty/v2"
)

type testBase struct {
	pr             *perfRunner
	workerID       int
	actionsPerLoop int
}

func (t *testBase) WorkerID() int {
	return t.workerID
}

func (t *testBase) ActionsPerLoop() int {
	return t.actionsPerLoop
}

func resStatus(res *resty.Response) int {
	if res == nil {
		return -1
	}
	return res.StatusCode()
}
