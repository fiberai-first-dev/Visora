package events

import (
	"encoding/json"
	"fmt"

	"visora-backend/internal/db"
)

// PublishEvent creates a ScanEvent record for SSE stream.
//
// data is serialized to a JSON document. Passing a non-nil error in place of data
// is not supported: callers should pass structured values only.
func PublishEvent(projectID uint, stage int, eventType, message string, data interface{}) {
	dataJSON := "{}"
	if data != nil {
		if b, err := json.Marshal(data); err == nil && string(b) != "null" {
			dataJSON = string(b)
		}
	}
	if err := db.DB.Create(&db.ScanEvent{
		ProjectID: projectID,
		Stage:     stage,
		EventType: eventType,
		Message:   message,
		Data:      db.RawJSON(dataJSON),
	}).Error; err != nil {
		fmt.Printf("events: failed to publish stage %d event for project %d: %v\n", stage, projectID, err)
	}
}
