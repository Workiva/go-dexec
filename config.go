package dexec

import (
	"github.com/newrelic/go-agent/v3/newrelic"
	"github.com/sirupsen/logrus"
	"time"
)

type Config struct {
	ContainerConfig ContainerConfig
	NetworkConfig   NetworkConfig
	TaskConfig      TaskConfig
	CommandDetails  CommandDetails
	Logger          *logrus.Entry
	NewRelic        *newrelic.Application
}

type Mount struct {
	Type        string
	Source      string
	Destination string
	Options     []string
}

type ContainerConfig struct {
	Image  string
	User   string
	Env    []string
	Mounts []Mount
}

type TaskConfig struct {
	Executable string
	Args       []string
	Timeout    time.Duration
	WorkingDir string
}

type NetworkConfig struct {
	DNS        []string
	DNSSearch  []string
	DNSOptions []string
}

type CommandDetails struct {
	ExecutorId      int64
	ChainExecutorId int64
	ResultId        int64
}
