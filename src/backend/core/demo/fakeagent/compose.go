// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package fakeagent

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/demo/estate"
)

type composeFile struct {
	Services map[string]struct {
		Image       string   `yaml:"image"`
		Command     any      `yaml:"command"`
		Environment any      `yaml:"environment"`
		Ports       []any    `yaml:"ports"`
		Volumes     []string `yaml:"volumes"`
		Deploy      struct {
			Replicas int `yaml:"replicas"`
		} `yaml:"deploy"`
		Restart string `yaml:"restart"`
	} `yaml:"services"`
}

// handleCompose simulates docker compose for the project the stacks module
// deploys: up creates or starts the project's containers, down removes them,
// and the lifecycle verbs move their state, so a stack page reflects the action.
func (s *session) handleCompose(msg *coreagent.WSMessage) {
	res := &coreagent.ComposePayload{Success: true}
	defer s.reply(&coreagent.WSMessage{Type: coreagent.MsgComposeResult, ID: msg.ID, Compose: res})
	p := msg.Compose
	if p == nil || p.ProjectName == "" || len(p.Args) == 0 {
		res.Success = false
		res.Error = "invalid compose request"
		return
	}
	host := s.agent.cfg.Host
	project := p.ProjectName
	verb := p.Args[0]
	var out strings.Builder
	ids := host.ProjectContainers(project)
	each := func(label string, op func(string) (estate.Container, error)) {
		for _, id := range ids {
			c, err := op(id)
			if err != nil {
				continue
			}
			fmt.Fprintf(&out, " Container %s  %s\n", c.Name, label)
		}
	}
	time.Sleep(time.Duration(300+len(ids)*40) * time.Millisecond)
	switch verb {
	case "up":
		if text, ok := p.Files["docker-compose.yml"]; ok {
			host.PutStack(estate.Stack{Name: project, Compose: text, Env: p.Env})
			if err := reconcileProject(host, project, text, &out); err != nil {
				res.Success = false
				res.Error = err.Error()
				return
			}
			ids = host.ProjectContainers(project)
		}
		each("Started", host.StartContainer)
	case "start":
		each("Started", host.StartContainer)
	case "stop":
		each("Stopped", host.StopContainer)
	case "restart":
		each("Restarted", host.RestartContainer)
	case "pause":
		each("Paused", host.PauseContainer)
	case "unpause":
		each("Unpaused", host.UnpauseContainer)
	case "down", "rm":
		for _, id := range ids {
			c, err := host.Container(id)
			if err != nil {
				continue
			}
			if err := host.RemoveContainer(id, true); err == nil {
				fmt.Fprintf(&out, " Container %s  Removed\n", c.Name)
			}
		}
		if err := host.RemoveNetwork(project + "_default"); err == nil {
			fmt.Fprintf(&out, " Network %s_default  Removed\n", project)
		}
	case "pull":
		if st := host.Stack(project); st != nil {
			for _, img := range imagesOf(st.Compose) {
				host.PullImage(img)
				fmt.Fprintf(&out, " %s Pulled\n", img)
			}
		}
	case "ps":
		for _, id := range ids {
			c, err := host.Container(id)
			if err != nil {
				continue
			}
			line, err := json.Marshal(map[string]any{
				"ID": c.ID[:12], "Name": c.Name, "Image": c.Image, "Service": c.Labels["com.docker.compose.service"],
				"State": string(c.State), "Status": string(c.State), "Project": project,
			})
			if err == nil {
				out.Write(line)
				out.WriteByte('\n')
			}
		}
	case "config":
		if st := host.Stack(project); st != nil {
			out.WriteString(st.Compose)
		}
	case "logs":
		for _, id := range ids {
			c, err := host.Container(id)
			if err != nil {
				continue
			}
			for _, l := range host.LogLines(c, time.Now(), 5) {
				fmt.Fprintf(&out, "%s  | %s\n", c.Name, l.Text)
			}
		}
	default:
		res.Success = false
		res.Error = "compose " + verb + " is not available in demo mode"
		return
	}
	res.Output = out.String()
}

func imagesOf(text string) []string {
	var f composeFile
	if err := yaml.Unmarshal([]byte(text), &f); err != nil {
		return nil
	}
	var out []string
	for _, svc := range f.Services {
		if svc.Image != "" {
			out = append(out, svc.Image)
		}
	}
	sort.Strings(out)
	return out
}

// reconcileProject creates any service container the compose file declares
// and the project does not have yet, pulling its image first like compose does.
func reconcileProject(host *estate.Host, project, text string, out *strings.Builder) error {
	var f composeFile
	if err := yaml.Unmarshal([]byte(text), &f); err != nil {
		return fmt.Errorf("yaml: %w", err)
	}
	if len(f.Services) == 0 {
		return fmt.Errorf("no services defined in the compose file")
	}
	existing := map[string]bool{}
	for _, id := range host.ProjectContainers(project) {
		if c, err := host.Container(id); err == nil {
			existing[c.Name] = true
		}
	}
	netName := project + "_default"
	if _, _, err := host.Network(netName); err != nil {
		if _, cerr := host.CreateNetwork(netName, "bridge", false, map[string]string{"com.docker.compose.project": project, "com.docker.compose.network": "default"}); cerr == nil {
			fmt.Fprintf(out, " Network %s  Created\n", netName)
		}
	}
	names := make([]string, 0, len(f.Services))
	for name := range f.Services {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		svc := f.Services[name]
		if svc.Image == "" {
			continue
		}
		host.PullImage(svc.Image)
		replicas := max(1, svc.Deploy.Replicas)
		for i := 1; i <= replicas; i++ {
			cname := fmt.Sprintf("%s-%s-%d", project, name, i)
			if existing[cname] {
				continue
			}
			restart := svc.Restart
			if restart == "" {
				restart = "no"
			}
			c := &estate.Container{
				Name:          cname,
				Image:         svc.Image,
				Command:       commandText(svc.Command),
				Profile:       "api",
				CPUBase:       3,
				MemBase:       128 << 20,
				MemLimit:      1 << 30,
				RestartPolicy: restart,
				Networks:      map[string]string{netName: fmt.Sprintf("172.30.%d.%d", estate.StableHash(project)%250, 2+i)},
				Labels: map[string]string{
					"com.docker.compose.project":          project,
					"com.docker.compose.service":          name,
					"com.docker.compose.container-number": fmt.Sprint(i),
					"com.docker.compose.oneoff":           "False",
					"com.docker.compose.version":          "2.29.7",
				},
			}
			for _, v := range svc.Volumes {
				parts := strings.Split(v, ":")
				if len(parts) >= 2 {
					vol := parts[0]
					kind := "bind"
					if !strings.HasPrefix(vol, "/") && !strings.HasPrefix(vol, ".") {
						vol = project + "_" + vol
						kind = "volume"
						host.CreateVolume(vol, "local", map[string]string{"com.docker.compose.project": project})
					}
					c.Mounts = append(c.Mounts, estate.MountPoint{Type: kind, Name: vol, Source: vol, Destination: parts[1], RW: true})
				}
			}
			if _, err := host.AddContainer(c); err == nil {
				fmt.Fprintf(out, " Container %s  Created\n", cname)
			}
		}
	}
	return nil
}

// DeployProject is the simulated `compose up` without an agent session. The
// estate is rebuilt on every boot, so the demo seed calls it to stand back up
// the stacks demo.db records as deployed, before any agent connects.
func DeployProject(host *estate.Host, project, compose string) error {
	if host == nil {
		return fmt.Errorf("no demo host for project %s", project)
	}
	var out strings.Builder
	host.PutStack(estate.Stack{Name: project, Compose: compose})
	if err := reconcileProject(host, project, compose, &out); err != nil {
		return err
	}
	for _, id := range host.ProjectContainers(project) {
		if _, err := host.StartContainer(id); err != nil {
			continue
		}
	}
	return nil
}

func commandText(v any) string {
	switch c := v.(type) {
	case string:
		return c
	case []any:
		parts := make([]string, 0, len(c))
		for _, p := range c {
			parts = append(parts, fmt.Sprint(p))
		}
		return strings.Join(parts, " ")
	}
	return ""
}
