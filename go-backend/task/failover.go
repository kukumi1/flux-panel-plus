package task

import (
	"flux-panel/go-backend/service"
	"time"
)

const failoverInterval = 30 * time.Second

func StartFailoverTask() {
	go func() {
		for {
			time.Sleep(failoverInterval)
			service.EvaluateFailover()
		}
	}()
}
