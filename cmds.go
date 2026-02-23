package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

type Command struct {
	Path          string   `json:"command_path"`
	Args          []string `json:"command_args"`
	AllowExitCode int      `json:"exit_code"`
	Event         string   `json:"event"`
	MsgFormat     string   `json:"msg_format"`
	ExtraAgents   []string `json:"extra_agents"`
}

func (c *Command) Run(fp string) bool {
	cmd := &exec.Cmd{}
	if len(c.Args) == 0 {
		msg := fmt.Sprintf(c.MsgFormat, fp)
		log.Printf("running %q\n", fmt.Sprintf("%s %s", c.Path, msg))
		cmd = exec.Command(c.Path, msg)
	} else {
		log.Printf("running %q\n", fmt.Sprintf("%s %s", c.Path, strings.Join(c.Args, " ")))
		cmd = exec.Command(c.Path, c.Args...)
	}

	err := cmd.Start()
	if err != nil {
		log.Println(err)
		return false
	}

	err = cmd.Wait()
	if err != nil {
		exit, ok := err.(*exec.ExitError)
		if !ok {
			return false
		}
		if exit.ExitCode() == c.AllowExitCode {
			return true
		}
		return false
	}

	return true
}

type Config struct {
	Commands    []Command `json:"commands"`
	ExtraAgents []string  `json:"extra_agents"`
}

func (cs Config) Get(event string) *Command {
	for _, c := range cs.Commands {
		if c.Event == event {
			return &c
		}
	}
	return nil
}

func LoadConfig(p string) Config {
	cmds := Config{}
	data, err := os.ReadFile(p)
	if err != nil {
		log.Fatal(err)
	}
	err = json.Unmarshal(data, &cmds)
	if err != nil {
		log.Fatal(err)
	}

	return cmds
}
