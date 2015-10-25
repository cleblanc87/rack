package main

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/convox/rack/Godeps/_workspace/src/github.com/aws/aws-sdk-go/aws"
	"github.com/convox/rack/Godeps/_workspace/src/github.com/aws/aws-sdk-go/service/kinesis"
	docker "github.com/convox/rack/Godeps/_workspace/src/github.com/fsouza/go-dockerclient"
)

type Monitor struct {
	client *docker.Client
	lock   sync.Mutex
	lines  map[string][][]byte
}

func NewMonitor() *Monitor {
	client, err := docker.NewClient(os.Getenv("DOCKER_HOST"))

	if err != nil {
		log.Fatal(err)
	}

	return &Monitor{
		client: client,
		lines:  make(map[string][][]byte),
	}
}

func (m *Monitor) Listen() {
	containers, err := m.client.ListContainers(docker.ListContainersOptions{})

	if err != nil {
		log.Fatal(err)
	}

	for _, container := range containers {
		m.handleCreate(container.ID)
	}

	ch := make(chan *docker.APIEvents)

	go m.handleEvents(ch)
	go m.streamLogs()

	m.client.AddEventListener(ch)

	for {
		time.Sleep(60 * time.Second)
	}
}

func (m *Monitor) handleEvents(ch chan *docker.APIEvents) {
	for event := range ch {
		switch event.Status {
		case "create":
			m.handleCreate(event.ID)
		case "die":
			m.handleDie(event.ID)
		case "start":
			m.handleStart(event.ID)
		}
	}
}

func (m *Monitor) handleCreate(id string) {
	env, err := m.inspectContainerEnv(id)

	if err != nil {
		log.Printf("error: %s\n", err)
		return
	}

	go m.subscribeLogs(id, env["KINESIS"], env["PROCESS"])
}

func (m *Monitor) handleDie(id string) {
}

func (m *Monitor) handleStart(id string) {
	env, err := m.inspectContainerEnv(id)

	if err != nil {
		log.Printf("error: %s\n", err)
		return
	}

	m.updateCgroups(id, env)
}

func (m *Monitor) inspectContainerEnv(id string) (map[string]string, error) {
	env := map[string]string{}

	container, err := m.client.InspectContainer(id)

	if err != nil {
		log.Printf("error: %s\n", err)
		return env, err
	}

	for _, e := range container.Config.Env {
		parts := strings.SplitN(e, "=", 2)

		if len(parts) == 2 {
			env[parts[0]] = parts[1]
		}
	}

	return env, nil
}

func (m *Monitor) updateCgroups(id string, env map[string]string) {
	if env["SWAP"] == "1" {
		bytes := "18446744073709551615"

		fmt.Fprintf(os.Stderr, "id=%s cgroup=memory.memsw.limit_in_bytes value=%s\n", id, bytes)
		err := ioutil.WriteFile(fmt.Sprintf("/cgroup/memory/docker/%s/memory.memsw.limit_in_bytes", id), []byte(bytes), 0644)

		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
		}

		fmt.Fprintf(os.Stderr, "id=%s cgroup=memory.soft_limit_in_bytes value=%s\n", id, bytes)
		err = ioutil.WriteFile(fmt.Sprintf("/cgroup/memory/docker/%s/memory.soft_limit_in_bytes", id), []byte(bytes), 0644)

		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
		}

		fmt.Fprintf(os.Stderr, "id=%s cgroup=memory.limit_in_bytes value=%s\n", id, bytes)
		err = ioutil.WriteFile(fmt.Sprintf("/cgroup/memory/docker/%s/memory.limit_in_bytes", id), []byte(bytes), 0644)

		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
		}
	}
}

func (m *Monitor) subscribeLogs(id, stream, process string) {
	if stream == "" {
		return
	}

	time.Sleep(500 * time.Millisecond)

	r, w := io.Pipe()

	go func(prefix string, r io.ReadCloser) {
		defer r.Close()

		scanner := bufio.NewScanner(r)

		for scanner.Scan() {
			m.addLine(stream, []byte(fmt.Sprintf("%s: %s", process, scanner.Text())))
		}

		if scanner.Err() != nil {
			log.Printf("error: %s\n", scanner.Err())
		}
	}(process, r)

	err := m.client.Logs(docker.LogsOptions{
		Container:    id,
		Follow:       true,
		Stdout:       true,
		Stderr:       true,
		Tail:         "all",
		RawTerminal:  false,
		OutputStream: w,
		ErrorStream:  w,
	})

	if err != nil {
		log.Printf("error: %s\n", err)
	}

	w.Close()
}

func (m *Monitor) streamLogs() {
	Kinesis := kinesis.New(&aws.Config{})

	for _ = range time.Tick(100 * time.Millisecond) {
		for _, stream := range m.streams() {
			l := m.getLines(stream)

			if l == nil {
				continue
			}

			records := &kinesis.PutRecordsInput{
				Records:    make([]*kinesis.PutRecordsRequestEntry, len(l)),
				StreamName: aws.String(stream),
			}

			for i, line := range l {
				records.Records[i] = &kinesis.PutRecordsRequestEntry{
					Data:         line,
					PartitionKey: aws.String(string(time.Now().UnixNano())),
				}
			}

			res, err := Kinesis.PutRecords(records)

			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %s\n", err)
			}

			for _, r := range res.Records {
				if r.ErrorCode != nil {
					fmt.Printf("error: %s\n", *r.ErrorCode)
				}
			}

			fmt.Printf("upload to=kinesis stream=%q lines=%d\n", stream, len(res.Records))
		}
	}
}

func (m *Monitor) addLine(stream string, data []byte) {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.lines[stream] = append(m.lines[stream], data)
}

func (m *Monitor) getLines(stream string) [][]byte {
	m.lock.Lock()
	defer m.lock.Unlock()

	nl := len(m.lines[stream])

	if nl == 0 {
		return nil
	}

	if nl > 500 {
		nl = 500
	}

	ret := make([][]byte, nl)
	copy(ret, m.lines[stream])
	m.lines[stream] = m.lines[stream][nl:]

	return ret
}

func (m *Monitor) streams() []string {
	m.lock.Lock()
	defer m.lock.Unlock()

	streams := make([]string, len(m.lines))
	i := 0

	for key, _ := range m.lines {
		streams[i] = key
		i += 1
	}

	return streams
}
