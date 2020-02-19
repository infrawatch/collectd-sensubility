package sensu

import (
	"encoding/json"
	"reflect"
	"time"

	"github.com/infrawatch/collectd-sensubility/config"
	"github.com/infrawatch/collectd-sensubility/logging"
)

type Check struct {
	Command      string   `json:"command"`
	Subscribers  []string `json:"subscribers"`
	Interval     int      `json:"interval"`
	Timeout      int      `json:"timeout"`
	Ttl          int      `json:"ttl"`
	Ttl_status   int      `json:"ttl_status"`
	Occurrences  int      `json:"occurrences"`
	Refresh      int      `json:"refresh"`
	Handlers     []string `json:"handlers"`
	Dependencies []string `json:"dependencies"`
}

type Scheduler struct {
	Checks map[string]Check
	log    *logging.Logger
}

func NewScheduler(cfg *config.Config, logger *logging.Logger) (*Scheduler, error) {
	var scheduler Scheduler
	scheduler.log = logger
	err := json.Unmarshal(cfg.Sections["sensu"].Options["checks"].GetBytes(), &scheduler.Checks)
	if err != nil {
		return nil, err
	}
	return &scheduler, nil
}

// Schedules tickers to each check which will send the results to outchan.
func (self *Scheduler) Start(outchan chan interface{}) {
	// dynamically create select cases together with corresponding check names
	checks := []string{}
	cases := []reflect.SelectCase{}
	for name, data := range self.Checks {
		if data.Interval < 1 {
			self.log.Metadata(map[string]interface{}{"check": name, "interval": data.Interval})
			self.log.Warn("Configuration contains invalid interval.")
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
			self.log.Metadata(map[string]interface{}{"check": checks[index]})
			self.log.Debug("Requesting execution of check.")
			outchan <- CheckRequest{
				Command: self.Checks[checks[index]].Command,
				Name:    checks[index],
				Issued:  time.Now().Unix(),
			}
		}
	}()
}
