package formats

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/infrawatch/apputils/connector"
	"github.com/infrawatch/collectd-sensubility/sensu"
)

//global indentifiers
var (
	DefaultHostUUID = (uuid.New()).String()
)

//SGResult is a format of event which Smart Gateway from STF understands.
//Channel for collectd events has to be used.
type SGResult struct {
	Labels      map[string]string      `json:"labels"`
	Annotations map[string]interface{} `json:"annotations"`
	StartsAt    string                 `json:"startsAt"`
}

//VESEvent is root object for VES event
type VESEvent struct {
	Header    VESEventHeader `json:"commonEventHeader"`
	HeartBeat VESHeartBeat   `json:"heartbeatFields"`
}

//VESEventHeader holds common data about event
type VESEventHeader struct {
	Domain    string `json:"domain"`
	EventType string `json:"eventType"`
	EventID   string `json:"eventId"`

	Priority              string `json:"priority"`
	ReportingEntityID     string `json:"reportingEntityId"`
	ReportingEntityName   string `json:"reportingEntityName"`
	SourceID              string `json:"sourceId"`
	SourceName            string `json:"sourceName"`
	StartingEpochMicrosec int64  `json:"startingEpochMicrosec"`
	LastEpochMicrosec     int64  `json:"lastEpochMicrosec"`
}

//VESHeartBeat holds data related to heart beat event
type VESHeartBeat struct {
	AdditionalFields map[string]string `json:"additionalFields"`
}

func buildVESPriority(checkResult connector.CheckResult) string {
	if checkResult.Result.Status == sensu.ExitCodeSuccess {
		return "Normal"
	}
	return "High"
}

//CreateSGResult formats Sensu result so that Smart Gateway understands it
func CreateSGResult(input connector.CheckResult) (SGResult, error) {
	output := SGResult{
		Labels:      make(map[string]string),
		Annotations: make(map[string]interface{}),
		StartsAt:    (time.Now()).Format(time.RFC3339),
	}
	// format collectd labes
	output.Labels["client"] = input.Client
	output.Labels["check"] = input.Result.Name
	if input.Result.Status == sensu.ExitCodeSuccess {
		output.Labels["severity"] = "OKAY"
	} else if input.Result.Status == sensu.ExitCodeWarning {
		output.Labels["severity"] = "WARNING"
	} else {
		output.Labels["severity"] = "FAILURE"
	}
	// format collectd annotations
	output.Annotations["command"] = input.Result.Command
	output.Annotations["issued"] = input.Result.Issued
	output.Annotations["executed"] = input.Result.Executed
	output.Annotations["duration"] = input.Result.Duration
	output.Annotations["output"] = input.Result.Output
	output.Annotations["status"] = input.Result.Status

	vesData, err := json.Marshal(VESEvent{
		Header: VESEventHeader{
			Domain:                "heartbeat",
			EventType:             "checkResult",
			EventID:               fmt.Sprintf("%s-%s", input.Client, input.Result.Name),
			Priority:              buildVESPriority(input),
			ReportingEntityID:     DefaultHostUUID,
			ReportingEntityName:   input.Client,
			SourceID:              DefaultHostUUID,
			SourceName:            fmt.Sprintf("%s-%s", input.Client, "collectd-sensubility"),
			StartingEpochMicrosec: input.Result.Executed,
			LastEpochMicrosec:     input.Result.Executed + int64(input.Result.Duration),
		},
		HeartBeat: VESHeartBeat{
			AdditionalFields: map[string]string{
				"check":    input.Result.Name,
				"command":  input.Result.Command,
				"issued":   fmt.Sprintf("%d", input.Result.Issued),
				"executed": fmt.Sprintf("%d", input.Result.Executed),
				"duration": fmt.Sprintf("%f", input.Result.Duration),
				"output":   input.Result.Output,
				"status":   fmt.Sprintf("%d", input.Result.Status),
			},
		},
	})
	output.Annotations["ves"] = string(vesData)
	return output, err
}
