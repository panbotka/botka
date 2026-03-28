package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
)

// runCommandArgs holds the arguments for the run_command tool.
type runCommandArgs struct {
	ProjectName string `json:"project_name"`
	Command     string `json:"command"`
}

// handleRunCommand executes a project's configured dev or deploy command.
func (s *Server) handleRunCommand(raw json.RawMessage) (interface{}, error) {
	if s.commands == nil {
		return nil, errors.New("command execution is not available in stdio mode")
	}

	var args runCommandArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if args.ProjectName == "" {
		return nil, errors.New("project_name is required")
	}
	if args.Command != "dev" && args.Command != "deploy" {
		return nil, errors.New("command must be \"dev\" or \"deploy\"")
	}

	project, err := s.findProjectByName(args.ProjectName)
	if err != nil {
		return nil, err
	}

	rc, err := s.commands.Run(&project, args.Command)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"pid":          rc.PID,
		"command_type": rc.CommandType,
	}, nil
}

// listCommandsArgs holds the arguments for the list_commands tool.
type listCommandsArgs struct {
	ProjectName string `json:"project_name"`
}

// handleListCommands lists running commands for a project.
func (s *Server) handleListCommands(raw json.RawMessage) (interface{}, error) {
	if s.commands == nil {
		return nil, errors.New("command tracking is not available in stdio mode")
	}

	var args listCommandsArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if args.ProjectName == "" {
		return nil, errors.New("project_name is required")
	}

	project, err := s.findProjectByName(args.ProjectName)
	if err != nil {
		return nil, err
	}

	return s.commands.List(project.ID), nil
}

// killCommandArgs holds the arguments for the kill_command tool.
type killCommandArgs struct {
	ProjectName string `json:"project_name"`
	PID         int    `json:"pid"`
}

// handleKillCommand kills a running command by PID.
func (s *Server) handleKillCommand(raw json.RawMessage) (interface{}, error) {
	if s.commands == nil {
		return nil, errors.New("command tracking is not available in stdio mode")
	}

	var args killCommandArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if args.ProjectName == "" {
		return nil, errors.New("project_name is required")
	}
	if args.PID <= 0 {
		return nil, errors.New("pid is required")
	}

	// Validate project exists.
	if _, err := s.findProjectByName(args.ProjectName); err != nil {
		return nil, err
	}

	if !s.commands.Kill(args.PID) {
		return nil, errors.New("command not found")
	}

	return "Command killed.", nil
}
