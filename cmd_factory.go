package dexec

import (
	"context"
	"errors"
	"fmt"
	"github.com/containerd/containerd"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/opencontainers/runtime-spec/specs-go"
)

func Command(client interface{}, config Config) Cmd {
	switch c := client.(type) {
	case *docker.Client:
		dc := Docker{Client: c}
		execution := getDockerExecution(config)
		cmd := dc.Command(execution, config.TaskConfig.Executable, config.TaskConfig.Args...)
		cmd.NewRelic = config.NewRelic
		return cmd
	case *containerd.Client:
		if config.Namespace == "" {
			panic(errors.New("config must must have namespace set"))
		}
		cdc := Containerd{ContainerdClient: c, Namespace: config.Namespace}
		execution := getContainerdExecution(config)
		cmd := cdc.Command(execution, config.TaskConfig.Executable, config.TaskConfig.Args...)
		cmd.NewRelic = config.NewRelic
		return cmd
	default:
		panic(fmt.Errorf("unsupported client type: %v", c))
	}
}

func getDockerExecution(config Config) Execution[Docker] {
	exec, _ := ByCreatingContainer(docker.CreateContainerOptions{
		Config: &docker.Config{
			Image:        config.ContainerConfig.Image,
			AttachStdout: true,
			AttachStderr: true,
			User:         config.ContainerConfig.User,
			Env:          config.ContainerConfig.Env,
		},
		HostConfig: &docker.HostConfig{
			DNS:        config.NetworkConfig.DNS,
			DNSSearch:  config.NetworkConfig.DNSSearch,
			DNSOptions: config.NetworkConfig.DNSOptions,
			Mounts:     convertMounts[docker.HostMount](config.ContainerConfig.Mounts),
		},
		Context: context.Background(),
	})
	return exec
}

func getContainerdExecution(config Config) Execution[Containerd] {
	exec, _ := ByCreatingTask(CreateTaskOptions{
		Image:          config.ContainerConfig.Image,
		Mounts:         convertMounts[specs.Mount](config.ContainerConfig.Mounts),
		User:           config.ContainerConfig.User,
		Env:            config.ContainerConfig.Env,
		CommandTimeout: config.TaskConfig.Timeout,
		WorkingDir:     config.TaskConfig.WorkingDir,
		CommandDetails: config.CommandDetails,
	}, config.Logger)
	return exec
}

type mountable interface {
	docker.HostMount | specs.Mount
}

func convertMounts[T mountable](ms []Mount) []T {
	mounts := make([]T, len(ms))
	for i, mount := range ms {
		mounts[i] = convertMount[T](mount)
	}
	return mounts
}
func convertMount[T mountable](m Mount) T {
	var res T
	switch v := any(&res).(type) {
	case *docker.HostMount:
		*v = docker.HostMount{
			Type:     m.Type,
			Source:   m.Source,
			Target:   m.Destination,
			ReadOnly: isReadOnly(m),
		}
	case *specs.Mount:
		*v = specs.Mount{
			Type:        m.Type,
			Source:      m.Source,
			Destination: m.Destination,
			Options:     m.Options,
		}
	}
	return res
}

func isReadOnly(m Mount) bool {
	for _, opt := range m.Options {
		if opt == "ro" {
			return true
		}
	}
	return false
}
