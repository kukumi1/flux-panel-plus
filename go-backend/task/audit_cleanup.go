package task

import (
	"flux-panel/go-backend/model"
	"log"
	"time"

	"gorm.io/gorm"
)

const (
	auditMaxRowsXui  = 300 // x-ui shipper (node_id 0)
	auditMaxRowsNode = 200 // all flux nodes combined
)

func StartAuditCleanupTask(db *gorm.DB) {
	go func() {
		for {
			time.Sleep(30 * time.Second)
			trimConnectionAudit(db)
		}
	}()
	go func() {
		for {
			now := time.Now()
			next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 5, 0, now.Location())
			time.Sleep(time.Until(next))
			if err := db.Exec("DELETE FROM connection_audit").Error; err != nil {
				log.Printf("[AuditCleanup] daily clear failed: %v", err)
				continue
			}
			log.Println("[AuditCleanup] daily clear done")
		}
	}()
}

// trimConnectionAudit caps rows per source so the audit view stays real-time and
// the table never grows unbounded: the x-ui shipper (node_id 0) keeps the most
// recent auditMaxRowsXui rows, while all flux nodes share auditMaxRowsNode rows.
func trimConnectionAudit(db *gorm.DB) {
	trimAuditBucket(db, "node_id = ?", auditMaxRowsXui)
	trimAuditBucket(db, "node_id <> ?", auditMaxRowsNode)
}

func trimAuditBucket(db *gorm.DB, nodeCond string, maxRows int) {
	var threshold int64
	err := db.Model(&model.ConnectionAudit{}).
		Where(nodeCond, 0).
		Select("id").Order("id DESC").Offset(maxRows - 1).Limit(1).
		Scan(&threshold).Error
	if err != nil || threshold == 0 {
		return
	}
	db.Where(nodeCond, 0).Where("id < ?", threshold).Delete(&model.ConnectionAudit{})
}
