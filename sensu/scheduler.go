package sensu

import (
	"encoding/json"
	"reflect"
	"time"

	"github.com/infrawatch/apputils/config"
	connector "github.com/infrawatch/apputils/connector/sensu"
	"github.com/infrawatch/apputils/logging"
)

// Check holds data for single Sensu check to be scheduled
type Check struct {
	Command      string   `json:"command"`
	Subscribers  []string `json:"subscribers"`
	Interval     int      `json:"interval"`
	Timeout      int      `json:"timeout"`
	TTL          int      `json:"ttl"`
	TTLStatus    int      `json:"ttl_status"`
	Occurrences  int      `json:"occurrences"`
	Refresh      int      `json:"refresh"`
	Handlers     []string `json:"handlers"`
	Dependencies []string `json:"dependencies"`
}

// Scheduler holds data for scheduling standaline checks
type Scheduler struct {
	Checks map[string]Check
	log    *logging.Logger
}

// NewScheduler creates Sensu standalone check scheduler according to configuration
func NewScheduler(cfg *config.INIConfig, logger *logging.Logger) (*Scheduler, error) {
	var scheduler Scheduler
	scheduler.log = logger
	err := json.Unmarshal(cfg.Sections["sensu"].Options["checks"].GetBytes(), &scheduler.Checks)
	if err != nil {
		return nil, err
	}
	return &scheduler, nil
}

// Start schedules tickers to each check which will send the results to outchan.
func (sched *Scheduler) Start(outchan chan interface{}) {
	// dynamically create select cases together with corresponding check names
	checks := []string{}
	cases := []reflect.SelectCase{}
	for name, data := range sched.Checks {
		if data.Interval < 1 {
			sched.log.Metadata(map[string]interface{}{"check": name, "interval": data.Interval})
			sched.log.Warn("Configuration contains invalid interval.")
			continue
		}
		//TODO: use rather time.NewTicker() to be able to ticker.Stop() all tickers in Scheduler.Stop()
		cases = append(cases, reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(time.Tick(time.Duration(data.Interval) * time.Second)),
		})
		checks = append(checks, name)
	}
	// dynamic select
	go func() {
		for {
			index, _, _ := reflect.Select(cases)
			// request check execution
			sched.log.Metadata(map[string]interface{}{"check": checks[index]})
			sched.log.Debug("Requesting execution of check.")
			outchan <- connector.CheckRequest{
				Command: sched.Checks[checks[index]].Command,
				Name:    checks[index],
				Issued:  time.Now().Unix(),
			}
		}
	}()
}
