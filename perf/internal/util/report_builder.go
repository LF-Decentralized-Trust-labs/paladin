package util

import (
	"fmt"
	"html/template"
	"os"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

type TestRunMetrics struct {
	Name         string
	TotalActions string
	Duration     string
	SendRate     string
	MinLatency   string
	MaxLatency   string
	AvgLatency   string
	Throughput   string
}
type Report struct {
	RunnerConfig     string
	TestInstanceName string
	TestRuns         []TestRunMetrics
}

func (r *Report) GenerateHTML() error {
	htmlTemplate := `<!DOCTYPE html>
    <html lang="en">
    
    <head>
        <meta charset="UTF-8">
        <meta name="viewport" content="width=device-width, initial-scale=1.0">
        <title>Paladin: Performance Report</title>
        <style>
            html,
            body {
                height: 100%;
            }

            .main {
                background: #fff;
                display: flex;
                height: 100%;
            }

            .sidebar {
                background: #fafafa;
                border-right: 1px solid #e0e0e0;
                min-height: 100%;
                padding: 30px;
                margin-right: 30px;
                width: 260px;
            }

            .sidebar-logo {
                display: block;
                height: auto;
                margin: 0 auto 20px auto;
                width: 160px;
            }

            .sidebar-list {
                list-style: none;
                margin: 0;
                padding: 0;
            }

            .sidebar-list-item {
                margin: 10px 0;
            }

            .sidebar-list-item-link {
                color: #000;
                text-decoration: none;
            }

            .sidebar-list-item-link:hover {
                color: #462de0;
            }

            .sidebar-sublist {
                border-left: 1px solid #e0e0e0;
                font-size: 13px;
                list-style: none;
                margin: 10px 0 0 0;
                padding: 0 0 0 10px;
            }

            .sidebar-sublist-item {
                margin: 5px 0;
            }

            .sidebar-sublist-item-link {
                color: #000;
                text-decoration: none;
            }

            .sidebar-sublist-item-link:hover {
                color: #462de0;
            }

            .content {
                box-sizing: border-box;
                padding-right: 30px;
                width: 80%;
            }

            table {
                font-size: 11px;
                color: #333333;
                border-width: 1px;
                border-color: #666666;
                border-collapse: collapse;
                margin-bottom: 10px;
                min-width: 100%;
            }

            th {
                border-width: 1px;
                font-size: small;
                padding: 8px;
                border-style: solid;
                border-color: #666666;
                background-color: #f2f2f2;
            }

            td {
                border-width: 1px;
                font-size: small;
                padding: 8px;
                border-style: solid;
                border-color: #666666;
                background-color: #ffffff;
                font-weight: 400;
                text-align: center;
            }

            body {
                font: 15px/20px Helvetica, Arial, sans-serif;
                margin: 0;
            }

            code {
                display: block;
                padding: 10px;
                background-color: #f4f4f4;
                border: 1px solid #ccc;
                font-size: 0.9em;
            }
        </style>
    </head>
    
    <body>
        <section class="main">
            <aside class="sidebar">
                <ul class="sidebar-list">
                    <li class="sidebar-list-item">
                        <a class="sidebar-list-item-link" href="#test-configuration">
                            Test Instance Configuration
                        </a>
                    </li>
                    <li class="sidebar-list-item">
                        <a class="sidebar-list-item-link" href="#test-metrics">
                            Test Metrics
                        </a>
                    </li>
                </ul>
            </aside>
            <main class="content">
                <h1>Paladin Performance Report</h1>
                <p>Version: v0.1.0</p>
                <p>Test instance name: {{.TestInstanceName}}</p>
                <section id="test-configuration">
                    <h2>Test Instance Configuration</h2>
                    <code>
                    <pre>{{.RunnerConfig}}</pre>
                </code>
                </section>
    
                <section id="test-metrics">
                    <h2>Test Metrics</h2>
                    <div>
                        <table>
                            <tr>
                                <th>Test name</th>
                                <th>Test duration (secs)</th>
                                <th>Actions</th>
                                <th>Send TPS</th>
                                <th>Min Latency</th>
                                <th>Max Latency</th>
                                <th>Avg Latency</th>
                                <th>Throughput</th>
                            </tr>
                            {{range .TestRuns}}
                            <tr>
                                <td>{{.Name}}</td>
                                <td>{{.Duration}}</td>
                                <td>{{.TotalActions}}</td>
                                <td>{{.SendRate}}</td>
                                <td>{{.MinLatency}}</td>
                                <td>{{.MaxLatency}}</td>
                                <td>{{.AvgLatency}}</td>
                                <td>{{.Throughput}}</td>
                            </tr>
                            {{end}}
                        </table>
                    </div>
                </section>
            </main>
        </section>
    </body>
    
    </html>`
	// Execute the template
	tmpl, err := template.New("template").Parse(htmlTemplate)
	if err != nil {
		return err
	}

	// Create or open the output file
	outputFile, err := os.Create("pldperf-report.html")
	if err != nil {
		return err
	}
	defer outputFile.Close()

	// Write the HTML output to the file
	err = tmpl.Execute(outputFile, r)
	if err != nil {
		return err
	}

	return nil
}

func (r *Report) AddTestRunMetrics(name string, totalActions int64, duration time.Duration, tps *TPS, lt *Latency) {
	r.TestRuns = append(r.TestRuns, TestRunMetrics{
		Name:         name,
		TotalActions: fmt.Sprintf("%d", totalActions),
		Duration:     fmt.Sprintf("%s", duration),
		SendRate:     fmt.Sprintf("%f", tps.SendRate),
		Throughput:   fmt.Sprintf("%f", tps.Throughput),
		MinLatency:   lt.Min().String(),
		MaxLatency:   lt.Max().String(),
		AvgLatency:   lt.Avg().String(),
	})
}

func NewReportForTestInstance(runnerConfig string, instanceName string) *Report {
	return &Report{
		RunnerConfig:     runnerConfig,
		TestInstanceName: instanceName,
		TestRuns:         make([]TestRunMetrics, 0),
	}
}

type TPS struct {
	SendRate   float64 `json:"sendRate"`
	Throughput float64 `json:"throughput"`
}

func GenerateTPS(totalActions int64, startTime int64, endSendTime int64) *TPS {
	sendDuration := time.Duration((endSendTime - startTime) * int64(time.Second))
	sendDurationSec := sendDuration.Seconds()
	sendRate := float64(totalActions) / sendDurationSec

	totalDurationSec := time.Since(time.Unix(startTime, 0)).Seconds()
	throughput := float64(totalActions) / totalDurationSec
	log.Infof("Send rate: %f, Throughput: %f, Measured Actions: %v Duration: %v (Send duration: %v)", sendRate, throughput, totalActions, sendDurationSec, totalDurationSec)
	return &TPS{
		SendRate:   sendRate,
		Throughput: throughput,
	}
}

type Latency struct {
	mux   sync.Mutex
	min   time.Duration
	max   time.Duration
	total int64
	count int64
}

func (lt *Latency) Record(latency time.Duration) {
	lt.mux.Lock()
	defer lt.mux.Unlock()
	if latency < lt.min || lt.min.Nanoseconds() == 0 {
		lt.min = latency
	}
	if latency > lt.max {
		lt.max = latency
	}
	lt.total += latency.Milliseconds()
	lt.count++
}

func (lt *Latency) Avg() time.Duration {
	return time.Duration((lt.total / lt.count) * int64(time.Millisecond))
}

func (lt *Latency) Min() time.Duration {
	return lt.min
}

func (lt *Latency) Max() time.Duration {
	return lt.max
}
func (lt *Latency) String() string {
	return fmt.Sprintf("min: %s, max: %s, avg: %s", lt.Min(), lt.Max(), lt.Avg())
}
