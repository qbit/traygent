package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
)

type Command struct {
	Path          string   `json:"command_path"`
	Args          []string `json:"command_args"`
	AllowExitCode int      `json:"exit_code"`
	Event         string   `json:"event"`
	MsgFormat     string   `json:"msg_format"`
}

func (c *Command) Run(fp string) bool {
	cmd := &exec.Cmd{}
	if len(c.Args) == 0 {
		cmd = exec.Command(c.Path, fmt.Sprintf(c.MsgFormat, fp))
	} else {
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
	}

	return true
}

type Commands []Command

func (cs Commands) Get(event string) *Command {
	for _, c := range cs {
		if c.Event == event {
			return &c
		}
	}
	return nil
}

func LoadCommands(p string) Commands {
	cmds := Commands{}
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
