package task

import (
	"flux-panel/go-backend/service"
	"log"
	"time"
)

func StartStatisticsTask() {
	// Record flow snapshots immediately on startup as baseline,
	// so the first hourly run can compute deltas.
	log.Println("[StatisticsTask] Recording initial flow snapshots...")
	service.RecordForwardFlowSnapshots()
	service.RecordXrayFlowSnapshots()
	service.RecordUserFlowSnapshots()

	go func() {
		for {
			now := time.Now()
			// Schedule at the top of each hour
			next := time.Date(now.Year(), now.Month(), now.Day(), now.Hour()+1, 0, 0, 0, now.Location())
			time.Sleep(time.Until(next))

			log.Println("[StatisticsTask] Recording hourly statistics...")
			service.RecordHourlyStatistics()
			log.Println("[StatisticsTask] Hourly statistics recorded")
		}
	}()
}
